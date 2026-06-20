# Cross-device setup: connect a remote Claude to persistor-server

Runbook for connecting a Claude agent on another machine (laptop, etc.) to the
Persistor remote MCP daemon (`persistor-server`) running on **devbox** over
Tailscale. This is the manual half of the P1 acceptance gate.

You are the agent on the **remote** machine. Work through the steps, run the
commands, and fill in the report template at the end. Most steps are pure
shell; only Step 6 needs a Claude Code session reload.

## What this is

`persistor-server` is a long-running HTTP daemon that exposes four memory tools
(`memory_search`, `memory_get`, `memory_write`, `brief`) over the MCP Streamable
HTTP transport. In this phase it is **tailnet-bound and unauthenticated** — it
serves a throwaway test tenant, never devbox's live memory. The goal of this
runbook is to prove a second device on the tailnet can list and call those tools.

## Prerequisite: the daemon must be running on devbox

This is done **on devbox**, not the remote machine. If `curl` in Step 2 fails,
ask the operator to run this (or run it yourself if you have a devbox shell):

```bash
# On devbox. Builds the daemon, then runs it bound to the Tailscale IP,
# serving a THROWAWAY tenant on the TEST database (never the live memory).
make -C /home/user/code/persistor build

mkdir -p /tmp/persistor-remote-test/memory/daily

DATABASE_URL="postgres://persistor:$(cat /tmp/.pgpw_persistor)@localhost:5432/persistor_test?sslmode=disable" \
PERSISTOR_NOTES_DIR=/tmp/persistor-remote-test \
PERSISTOR_LISTEN_ADDR="$(tailscale ip -4 | head -1):8088" \
/home/user/code/persistor/bin/persistor-server
```

The daemon logs the address it is listening on. Leave it running. It needs no
tenant — it resolves the tenant from the bearer token. In a SECOND devbox
terminal, mint a key for a throwaway tenant (the token is printed once — copy
it):

```bash
DATABASE_URL="postgres://persistor:$(cat /tmp/.pgpw_persistor)@localhost:5432/persistor_test?sslmode=disable" \
  /home/user/code/persistor/bin/persistor key create \
  --tenant 11111111-1111-1111-1111-111111111111 --label laptop
```

Give that token to the remote machine; the steps below send it as
`Authorization: Bearer <token>`.

## Step 1 - Find devbox on the tailnet

On the remote machine:

```bash
tailscale status | grep -i devbox
```

Take the `100.x.y.z` address from that line (or use the MagicDNS name
`devbox` if MagicDNS is enabled) and set it as a variable for the rest:

```bash
WARPCORE=100.x.y.z   # replace with the address from the line above
TOKEN=psk_...        # the token printed by `persistor key create` on devbox
```

## Step 2 - Confirm reachability

```bash
curl -sS "http://${WARPCORE}:8088/healthz" -o /dev/null -w '%{http_code}\n'
```

Expect `200`. If this hangs or refuses, the daemon is not running or is bound to
localhost instead of the tailnet IP — see Troubleshooting.

## Step 3 - Prove it is the persistor MCP server

```bash
curl -sS -X POST "http://${WARPCORE}:8088/mcp" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

The response is a server-sent-events frame (`event: message` then `data: {...}`).
The `data` JSON should contain `"serverInfo":{"name":"persistor",...}`. That
confirms the MCP endpoint is reachable and is Persistor. Drop the Authorization
header and you should get a `401` instead — that confirms auth is on.

## Step 4 - Add the MCP server to this machine

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer ${TOKEN}" \
  persistor-remote "http://${WARPCORE}:8088/mcp"
```

## Step 5 - Verify the tools are listed

```bash
claude mcp list
claude mcp get persistor-remote
```

`claude mcp list` should show `persistor-remote`; recent Claude Code versions
perform a live connection check and show it connected. If it shows the four
tools or a connected status, the wire-level gate is met.

## Step 6 - Functional round-trip

Restart Claude Code (or start a fresh session) so it loads the new MCP server,
then make these tool calls (ask the model to use the tools, or call them
directly if your harness allows):

1. `memory_write` — path `daily/laptop-check.md`, body something like
   `# Laptop check\n\nConnected from the laptop over Tailscale.`
2. `memory_search` — query `laptop`. Expect the note just written to come back.
3. `brief` — seed `laptop`. Expect a non-empty markdown working-set.

If all three succeed, the tools are callable end to end from this device.

## Step 7 - Report back

Reply to the operator with:

```text
Cross-device P1 check:
- Reachability (healthz 200): PASS / FAIL
- initialize returns serverInfo name=persistor: PASS / FAIL
- claude mcp list shows persistor-remote connected: PASS / FAIL
- memory_write -> memory_search round-trip: PASS / FAIL / not-tested
- Tailscale address used: 100.x.y.z (or MagicDNS name)
- Notes / anything odd:
```

## Step 8 - Clean up

```bash
claude mcp remove persistor-remote
```

Then stop the daemon on devbox with Ctrl-C in its terminal. The throwaway
tenant data lives only in the test database and can be ignored.

## Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `healthz` refused or hangs | daemon down, or bound to `127.0.0.1` not the tailnet | set `PERSISTOR_LISTEN_ADDR` to the Tailscale IP; check the daemon log |
| `403 Forbidden` | DNS-rebind protection | use the Tailscale IP or MagicDNS name in the URL, never `localhost` |
| refused only from the laptop | Tailscale down or ACLs block the port | `tailscale status` on both machines; allow TCP 8088 in tailnet ACLs |
| `404` on `/mcp` | wrong path | the URL must end in `/mcp` |
| `401 Unauthorized` | missing/invalid token | pass `--header "Authorization: Bearer <token>"`; mint with `persistor key create` |
| empty search results | nothing written yet | run `memory_write` first — there is no startup reindex |

## Security note

The daemon requires a per-tenant **bearer token** (a static API key): no token
or a bad one gets a `401`. Each token maps to one tenant, and tenants are
isolated by Postgres row-level security, so a token only ever reaches its own
tenant's memory. Keep the daemon tailnet-bound and off any public interface
until the hardening phase; real OAuth (for claude.ai and mobile) arrives in P3.
