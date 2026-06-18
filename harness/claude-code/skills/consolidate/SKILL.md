---
name: consolidate
description: Review queued session transcripts and write durable prose memory notes (the one LLM-judgment step of the memory system). Use when the user asks to "consolidate memory", at end of session, or to drain the consolidation queue.
---

# /consolidate — turn transcripts into durable prose memory

You are the **judgment step** of Persistor's memory. The
hooks and the Go CLI are deterministic plumbing; _deciding what is worth
remembering, and whether a new fact corrects an old one or adds a point to a
timeline, is the one irreducible LLM step._ Do it carefully.

## Inputs

- The consolidation queue: `${PERSISTOR_QUEUE:-~/.persistor/consolidation-queue.jsonl}`
  — one JSON line per ended session: `{transcript_path, cwd, session_id, reason, ts}`.
- Each `transcript_path` is the full session transcript (the durable fallback —
  notes can always be re-consolidated, so prefer omission over a bad note).

## Procedure

1. **Read the queue.** Take the un-drained entries (oldest first). If empty, stop
   and say so.
2. **For each transcript**, read it and extract only **durable** memory: facts,
   decisions, preferences, and notable episodes that will matter in a _future_
   session. Skip transient task chatter, anything already saved, and general
   world knowledge.
3. **Search before writing.** For each candidate note, call `memory_search` (or
   `persistor search`) to find an existing note it updates or duplicates.
   - **Correction** → the new fact replaces a stale one ("weighs 230 now" vs
     "weighs 250"). Write the correction at a **new path** and set `supersedes` to
     the old note's id. (Writing to the _same_ path is an in-place update with no
     history; a same-path write that supersedes its own id is rejected.)
   - **New point in a timeline** → both are true at their time ("190 lbs in
     2022"). Write a new note **without** `supersedes`; put the date in the prose.
   - **Duplicate** → skip it.
4. **Author each note as prose.** A clear title heading + a few sentences. Choose
   `kind` (fact|decision|episode|reference|preference) and `tier`:
   - `tail` (default) for the vast majority — retrieved on demand.
   - `core` ONLY for the small always-loaded surface; use it to keep a tight,
     curated Core summary rather than letting raw MEMORY.md bloat it.
5. **Emit a plan** (the deterministic apply contract — see
   `persistor consolidate --help`):

   ```json
   {
     "notes": [
       {
         "path": "project-x-status.md",
         "kind": "fact",
         "tier": "tail",
         "supersedes": "notes:project-x-2024",
         "body": "# Project X status\n\nCurrent status ...\n"
       }
     ]
   }
   ```

6. **Apply it deterministically:**

   ```bash
   echo "$PLAN" | persistor consolidate --plan -
   ```

   (Or write notes one at a time with the `memory_write` MCP tool.) The reindex
   that follows reconciles supersession automatically.

7. **Drain the queue** — remove the entries you processed (e.g. truncate the
   queue file) so they are not reconsolidated. Leave entries you skipped on
   purpose only if you intend to revisit them.

## Guardrails

- **Never fabricate.** Only write what the transcript supports.
- **Personal data stays local** — notes are written into the private notes repo /
  local DB, never anywhere public.
- **Prefer fewer, better notes.** The transcript is the fallback; a missed note
  is cheap to recover, a wrong note is expensive.
- **Supersede conservatively.** When unsure whether something corrects vs. adds,
  add (coexist) — don't hide history.
