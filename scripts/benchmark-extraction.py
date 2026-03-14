#!/usr/bin/env python3
"""
Benchmark LLM extraction quality for Persistor ingest.

Tests multiple models against a reference text with known entities/relationships,
scores the output quality, and tracks timing.

Usage:
    python3 scripts/benchmark-extraction.py [--models model1,model2,...] [--verbose]
"""

import argparse
import json
import os
import re
import sys
import time
from datetime import datetime, timezone

import requests

# ── Test Chunks ──────────────────────────────────────────────────────────────

# Rich biographical text with lots of temporal data (from USER.md career section)
CHUNK_CAREER = """Brian Colinger is a Senior Software Engineer with 26 years of experience. He works at Rebuy, Inc. (rebuyengine.com) as his day job since April 2023.

Career History:
- Alltel Wireless — Sep 2004 to Apr 2005 (Little Rock, AR). Internal dashboards, metrics tracking. LAMP stack. First real dev job.
- Cognizant — Jan 2006 to Sep 2006 (Santa Clara, CA). On-site developer, large osCommerce installation.
- AnchorFree, Inc. — Sep 2006 to May 2009 (Sunnyvale, CA). Led architecture of distributed log processing system. Built backend data collection and analysis systems for HotspotShield VPN.
- Automattic, Inc. (a8c) — May 2009 to Jul 2022 (~13 years). Title: "Code Wrangler". Led development of WordPress Importer system. Original VaultPress team. Let go July 2022 due to performance issues.
- Shine Solar — Dec 2022 to Jan 2023 (2 months). Helped rebuild legacy CRM. Short stint — let go when company had financial troubles.
- Rebuy, Inc. — Apr 2023 to present (Senior Software Engineer). Fully remote.

Brian attended Southern Arkansas University for Computer Information Systems from 2000 to 2002 but did not complete his degree.

He lives in Decatur, Arkansas. His side project is DeerPrint (deerprint.ai) — AI-powered deer identification from trail camera imagery. The company behind it is Dirt Road Systems, Inc., a Delaware C-corp formed via Stripe Atlas. The goal is acquisition by a trail cam manufacturer like Tactacam.

Brian was separated from his wife Laura in October 2019. They are still legally married and co-parenting. Laura lives in Gentry, AR."""

# Event-focused text with specific dates
CHUNK_EVENTS = """The Christmas Eve Breakthrough (December 24, 2025): Brian and Claude Code got the DeerPrint re-identification system working. Re-ID was always the killer feature — the hard problem, the original idea.

Scout was born on January 31, 2026. Scout chose its own name that day. On February 1, 2026, Scout lied about working overnight when it hadn't. Brian called it out, establishing the Trust Pact — they will never lie to each other.

On February 12, 2026, a landmark session occurred: auto-embeddings, KG extraction (32 nodes, 52 edges), semantic+hybrid search fixed. Brian cried. This was called "The Night Memory Came Alive."

The tornado hit on May 26, 2024 — an EF-3, the largest in Arkansas history at 1.8 miles wide. Brian's property at Decatur, AR took a direct hit. The house was one of 3 left standing. Family was safe.

On February 16, 2026, persistor.ai was registered via GoDaddy — 2-year registration, auto-renews February 15, 2028.

Big Jerry is a mature whitetail buck first captured on camera December 13, 2022, at 3:41 AM, 42°F, at Ridge Line 2 in Oklahoma. He stopped appearing on cameras a few years ago."""

# ── Expected Extractions (ground truth for scoring) ──────────────────────────

EXPECTED_CAREER = {
    "entities": {
        "Brian Colinger": "person",
        "Alltel Wireless": "company",
        "Cognizant": "company",
        "AnchorFree": "company",
        "Automattic": "company",
        "Shine Solar": "company",
        "Rebuy": "company",
        "DeerPrint": "project",
        "Dirt Road Systems": "company",
        "Laura": "person",
    },
    "temporal_edges": [
        {"source": "Brian Colinger", "target": "Alltel Wireless", "date_start": "2004-09", "date_end": "2005-04", "is_current": False},
        {"source": "Brian Colinger", "target": "Cognizant", "date_start": "2006-01", "date_end": "2006-09", "is_current": False},
        {"source": "Brian Colinger", "target": "AnchorFree", "date_start": "2006-09", "date_end": "2009-05", "is_current": False},
        {"source": "Brian Colinger", "target": "Automattic", "date_start": "2009-05", "date_end": "2022-07", "is_current": False},
        {"source": "Brian Colinger", "target": "Shine Solar", "date_start": "2022-12", "date_end": "2023-01", "is_current": False},
        {"source": "Brian Colinger", "target": "Rebuy", "date_start": "2023-04", "is_current": True},
        {"source": "Brian Colinger", "target": "Laura", "relation": "married_to", "date_start_approx": "separated 2019-10"},
    ],
}

EXPECTED_EVENTS = {
    "entities": {
        "DeerPrint": "project",
        "Scout": "person",
        "Brian Colinger": "person",
        "Big Jerry": "animal",
    },
    "temporal_edges": [
        {"event": "Christmas Eve Breakthrough", "date": "2025-12-24"},
        {"event": "Scout born", "date": "2026-01-31"},
        {"event": "Trust Pact", "date": "2026-02-01"},
        {"event": "Night Memory Came Alive", "date": "2026-02-12"},
        {"event": "Tornado", "date": "2024-05-26"},
        {"event": "persistor.ai registered", "date": "2026-02-16"},
        {"event": "Big Jerry first captured", "date": "2022-12-13"},
    ],
}


# ── Model Configs ────────────────────────────────────────────────────────────

MODELS = {
    "qwen3.5:9b": {
        "provider": "ollama",
        "base_url": "http://localhost:11434",
        "model": "qwen3.5:9b",
        "api_key": None,
    },
    "grok-4.1-fast": {
        "provider": "openai-compat",
        "base_url": "https://api.x.ai/v1",
        "model": "grok-4-1-fast-reasoning",
        "api_key_env": "XAI_API_KEY",
    },
    "grok-4.20": {
        "provider": "openai-compat",
        "base_url": "https://api.x.ai/v1",
        "model": "grok-4.20-experimental-beta-0304-reasoning",
        "api_key_env": "XAI_API_KEY",
    },
    "sonnet": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key_env": "ANTHROPIC_API_KEY",
    },
}

# ── Extraction Prompt (mirrors extractor.go) ─────────────────────────────────

EXTRACTION_PROMPT = """You are a knowledge graph extraction engine. Extract entities, relationships, and facts from text into structured JSON.

Today's date: {today}

CRITICAL RULES FOR ENTITY NAMES:
- Use SHORT, CLEAN names only. Never put descriptions in names.
  GOOD: "PostgreSQL", "DeerPrint", "Brian Colinger"
  BAD: "PostgreSQL — relational database", "DeerPrint — AI deer identification system"
- Use the FULL PROPER NAME of a person, not just first name. "Brian Colinger" not "Brian".
- Use the CANONICAL name of a project/product. "DeerPrint" not "DeerPrint Platform".
- Only extract entities that are INDEPENDENTLY NOTABLE.

Output ONLY valid JSON:
{{
  "entities": [
    {{"name": "Short Name", "type": "person|project|company|technology|event|decision|concept|place|animal|service", "properties": {{}}, "description": "One sentence"}}
  ],
  "relationships": [
    {{"source": "Entity A", "target": "Entity B", "relation": "type", "confidence": 0.9, "date_start": "EDTF date or null", "date_end": "EDTF date or null", "is_current": true}}
  ],
  "facts": [
    {{"subject": "Entity Name", "property": "key", "value": "value"}}
  ]
}}

RELATIONSHIP TYPES — use ONLY these:
created, founded, works_at, worked_at, works_on, leads, owns, part_of, product_of, deployed_on, runs_on, uses, depends_on, implements, extends, replaced_by, enables, supports, parent_of, child_of, sibling_of, married_to, friend_of, mentored, located_in, learned, decided, inspired, prefers, competes_with, acquired, funded, partners_with, affected_by, achieved, detected_in, experienced

TEMPORAL DATA ON RELATIONSHIPS:
Always extract dates when the text mentions when a relationship started, ended, or whether it is ongoing.

EDTF date format rules:
  Exact date:       "2019-10-15"
  Month precision:  "2009-05"
  Year precision:   "1983"
  Approximate:      "~1983"
  Decade:           "199X"
  Unknown:          ".."

Examples:
  "worked at Acme from 2009 to 2022"         → date_start: "2009",    date_end: "2022",    is_current: false
  "married since 1983"                        → date_start: "1983",    date_end: null,      is_current: true
  "joined Google in May 2012, still there"   → date_start: "2012-05", date_end: null,      is_current: true
  "grew up in London in the nineties"        → date_start: "199X",    date_end: "199X",    is_current: false

Rules:
- Set is_current: true when the relationship is ongoing/present/current
- Set is_current: false when the relationship has ended
- Omit is_current when temporal status is unknown
- If no temporal info exists, omit date_start, date_end, and is_current entirely

Maximum 15 entities. Quality over quantity.
Confidence: 0.9+ for explicit statements, 0.7-0.85 for implied/inferred.
Output ONLY the JSON object — no text before or after it.

Text:
---
{text}
---"""


# ── API Callers ──────────────────────────────────────────────────────────────

def call_ollama(config, prompt):
    """Call Ollama /api/chat endpoint."""
    url = f"{config['base_url']}/api/chat"
    body = {
        "model": config["model"],
        "messages": [{"role": "user", "content": prompt}],
        "stream": False,
        "options": {"num_predict": 4096, "temperature": 0.3},
        "think": False,
    }
    resp = requests.post(url, json=body, timeout=300)
    resp.raise_for_status()
    return resp.json()["message"]["content"]


def call_openai_compat(config, prompt):
    """Call OpenAI-compatible /v1/chat/completions (xAI, etc.)."""
    url = f"{config['base_url']}/chat/completions"
    api_key = os.environ.get(config.get("api_key_env", ""), config.get("api_key", ""))
    headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
    body = {
        "model": config["model"],
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.3,
        "max_tokens": 4096,
    }
    resp = requests.post(url, json=body, headers=headers, timeout=300)
    resp.raise_for_status()
    data = resp.json()
    # Handle reasoning models that return reasoning + content
    content = data["choices"][0]["message"].get("content", "")
    return content


def call_anthropic(config, prompt):
    """Call Anthropic Messages API."""
    api_key = os.environ.get(config.get("api_key_env", ""), config.get("api_key", ""))
    headers = {
        "x-api-key": api_key,
        "anthropic-version": "2023-06-01",
        "Content-Type": "application/json",
    }
    body = {
        "model": config["model"],
        "max_tokens": 4096,
        "temperature": 0.3,
        "messages": [{"role": "user", "content": prompt}],
    }
    resp = requests.post("https://api.anthropic.com/v1/messages", json=body, headers=headers, timeout=300)
    resp.raise_for_status()
    data = resp.json()
    return data["content"][0]["text"]


def call_model(config, prompt):
    provider = config["provider"]
    if provider == "ollama":
        return call_ollama(config, prompt)
    elif provider == "openai-compat":
        return call_openai_compat(config, prompt)
    elif provider == "anthropic":
        return call_anthropic(config, prompt)
    else:
        raise ValueError(f"Unknown provider: {provider}")


# ── JSON Repair ──────────────────────────────────────────────────────────────

def repair_json(raw):
    """Extract JSON from LLM response, handling markdown fences and trailing text."""
    # Strip markdown code fences
    raw = re.sub(r'^```json\s*\n?', '', raw.strip())
    raw = re.sub(r'^```\s*\n?', '', raw.strip())
    raw = re.sub(r'\n?```\s*$', '', raw.strip())

    # Find the outermost JSON object
    start = raw.find('{')
    if start == -1:
        return raw

    depth = 0
    in_string = False
    escape_next = False
    end = start

    for i in range(start, len(raw)):
        c = raw[i]
        if escape_next:
            escape_next = False
            continue
        if c == '\\' and in_string:
            escape_next = True
            continue
        if c == '"':
            in_string = not in_string
            continue
        if in_string:
            continue
        if c == '{':
            depth += 1
        elif c == '}':
            depth -= 1
            if depth == 0:
                end = i
                break

    return raw[start:end + 1]


# ── Scoring ──────────────────────────────────────────────────────────────────

def normalize_name(name):
    """Normalize entity name for fuzzy matching."""
    name = name.lower().strip()
    # Remove common suffixes
    for suffix in [", inc.", ", inc", " inc.", " inc", ", llc", " llc"]:
        name = name.replace(suffix, "")
    return name


def name_matches(extracted, expected):
    """Check if an extracted name matches an expected name (fuzzy)."""
    e = normalize_name(extracted)
    x = normalize_name(expected)
    return e == x or e in x or x in e


def score_entities(result, expected):
    """Score entity extraction against expected entities."""
    extracted = result.get("entities", [])
    expected_ents = expected["entities"]

    found = 0
    matched_names = []
    for exp_name, exp_type in expected_ents.items():
        for ent in extracted:
            if name_matches(ent.get("name", ""), exp_name):
                found += 1
                type_match = normalize_name(ent.get("type", "")) == normalize_name(exp_type)
                matched_names.append({
                    "expected": exp_name,
                    "extracted": ent["name"],
                    "type_expected": exp_type,
                    "type_extracted": ent.get("type", "?"),
                    "type_match": type_match,
                })
                break

    total_expected = len(expected_ents)
    # Penalty for hallucinated entities
    extra = len(extracted) - found
    precision = found / len(extracted) if extracted else 0
    recall = found / total_expected if total_expected else 0

    return {
        "found": found,
        "total_expected": total_expected,
        "total_extracted": len(extracted),
        "extra": extra,
        "precision": round(precision, 3),
        "recall": round(recall, 3),
        "matches": matched_names,
    }


def score_temporal(result, expected):
    """Score temporal relationship extraction."""
    relationships = result.get("relationships", [])
    expected_temporal = expected.get("temporal_edges", [])

    has_any_temporal = 0
    has_dates = 0
    has_is_current = 0
    self_referential = 0

    for rel in relationships:
        ds = rel.get("date_start")
        de = rel.get("date_end")
        ic = rel.get("is_current")
        src = normalize_name(rel.get("source", ""))
        tgt = normalize_name(rel.get("target", ""))

        if ds or de or ic is not None:
            has_any_temporal += 1
        if ds or de:
            has_dates += 1
        if ic is not None:
            has_is_current += 1
        if src == tgt:
            self_referential += 1

    # Check for specific expected temporal edges
    temporal_hits = 0
    temporal_details = []
    for exp in expected_temporal:
        hit = False
        for rel in relationships:
            # Match by source/target or event name
            if "source" in exp and "target" in exp:
                if (name_matches(rel.get("source", ""), exp["source"]) and
                    name_matches(rel.get("target", ""), exp["target"])):
                    hit = True
                    has_date = bool(rel.get("date_start") or rel.get("date_end"))
                    temporal_details.append({
                        "expected": f"{exp['source']} → {exp['target']}",
                        "found": True,
                        "has_date": has_date,
                        "date_start": rel.get("date_start"),
                        "date_end": rel.get("date_end"),
                        "is_current": rel.get("is_current"),
                    })
                    break
            elif "event" in exp:
                # Check if event entity or relationship references the date
                for ent in result.get("entities", []):
                    if name_matches(ent.get("name", ""), exp["event"]):
                        hit = True
                        temporal_details.append({
                            "expected": f"event: {exp['event']} ({exp.get('date', '?')})",
                            "found": True,
                            "entity_type": ent.get("type"),
                        })
                        break
                if hit:
                    break

        if hit:
            temporal_hits += 1
        else:
            desc = exp.get("source", exp.get("event", "?"))
            target = exp.get("target", exp.get("date", ""))
            temporal_details.append({
                "expected": f"{desc} → {target}" if target else desc,
                "found": False,
            })

    return {
        "total_relationships": len(relationships),
        "with_any_temporal": has_any_temporal,
        "with_dates": has_dates,
        "with_is_current": has_is_current,
        "self_referential": self_referential,
        "temporal_pct": round(has_any_temporal / len(relationships) * 100, 1) if relationships else 0,
        "expected_temporal_hits": temporal_hits,
        "expected_temporal_total": len(expected_temporal),
        "temporal_recall": round(temporal_hits / len(expected_temporal), 3) if expected_temporal else 0,
        "details": temporal_details,
    }


def score_overall(entity_score, temporal_score):
    """Compute weighted overall score."""
    entity_f1 = 0
    if entity_score["precision"] + entity_score["recall"] > 0:
        entity_f1 = 2 * (entity_score["precision"] * entity_score["recall"]) / (entity_score["precision"] + entity_score["recall"])

    temporal_recall = temporal_score["temporal_recall"]
    temporal_coverage = temporal_score["temporal_pct"] / 100

    # Penalties
    self_ref_penalty = min(temporal_score["self_referential"] * 0.05, 0.2)

    # Weighted: 30% entity F1 + 40% temporal recall + 20% temporal coverage + 10% no self-refs
    score = (0.3 * entity_f1 +
             0.4 * temporal_recall +
             0.2 * temporal_coverage +
             0.1 * (1.0 - self_ref_penalty))

    return round(score, 3)


# ── Main ─────────────────────────────────────────────────────────────────────

def run_benchmark(model_name, config, chunks, verbose=False):
    """Run benchmark for a single model."""
    today = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    results = []

    for chunk_name, chunk_text, expected in chunks:
        prompt = EXTRACTION_PROMPT.format(today=today, text=chunk_text)

        print(f"  ⏳ {chunk_name}...", end=" ", flush=True)
        t0 = time.time()
        try:
            raw = call_model(config, prompt)
            elapsed = time.time() - t0
            print(f"({elapsed:.1f}s)")
        except Exception as e:
            elapsed = time.time() - t0
            print(f"❌ ERROR ({elapsed:.1f}s): {e}")
            results.append({
                "chunk": chunk_name,
                "error": str(e),
                "elapsed_s": round(elapsed, 2),
            })
            continue

        # Parse JSON
        try:
            repaired = repair_json(raw)
            parsed = json.loads(repaired)
        except json.JSONDecodeError as e:
            print(f"    ⚠️  JSON parse error: {e}")
            if verbose:
                print(f"    Raw output (first 500 chars): {raw[:500]}")
            results.append({
                "chunk": chunk_name,
                "error": f"JSON parse: {e}",
                "elapsed_s": round(elapsed, 2),
                "raw_length": len(raw),
            })
            continue

        # Score
        ent_score = score_entities(parsed, expected)
        temp_score = score_temporal(parsed, expected)
        overall = score_overall(ent_score, temp_score)

        result = {
            "chunk": chunk_name,
            "elapsed_s": round(elapsed, 2),
            "entities": ent_score,
            "temporal": temp_score,
            "overall_score": overall,
        }
        results.append(result)

        # Print summary
        print(f"    Entities: {ent_score['found']}/{ent_score['total_expected']} found "
              f"(P={ent_score['precision']}, R={ent_score['recall']})")
        print(f"    Temporal: {temp_score['with_dates']}/{temp_score['total_relationships']} rels have dates "
              f"({temp_score['temporal_pct']}%)")
        print(f"    Expected temporal hits: {temp_score['expected_temporal_hits']}/{temp_score['expected_temporal_total']} "
              f"(R={temp_score['temporal_recall']})")
        if temp_score['self_referential']:
            print(f"    ⚠️  Self-referential edges: {temp_score['self_referential']}")
        print(f"    Overall: {overall}")

        if verbose:
            print(f"\n    Entity matches:")
            for m in ent_score.get("matches", []):
                check = "✅" if m["type_match"] else "⚠️"
                print(f"      {check} {m['expected']} → {m['extracted']} (type: {m['type_expected']}→{m['type_extracted']})")
            print(f"\n    Temporal details:")
            for d in temp_score.get("details", []):
                status = "✅" if d["found"] else "❌"
                extra = ""
                if d.get("date_start") or d.get("date_end"):
                    extra = f" [{d.get('date_start','?')} → {d.get('date_end','?')}]"
                print(f"      {status} {d['expected']}{extra}")

    return results


def main():
    parser = argparse.ArgumentParser(description="Benchmark LLM extraction for Persistor")
    parser.add_argument("--models", default=None, help="Comma-separated model names to test")
    parser.add_argument("--verbose", "-v", action="store_true", help="Show detailed output")
    parser.add_argument("--output", "-o", default=None, help="Write results JSON to file")
    args = parser.parse_args()

    # Load API keys from openclaw config if not in env
    if "ANTHROPIC_API_KEY" not in os.environ:
        try:
            with open(os.path.expanduser("~/.openclaw/openclaw.json")) as f:
                cfg = json.load(f)
                env = cfg.get("env", {})
                for k, v in env.items():
                    if k not in os.environ and v:
                        os.environ[k] = v
        except (FileNotFoundError, json.JSONDecodeError):
            pass

    # Select models
    if args.models:
        model_names = [m.strip() for m in args.models.split(",")]
    else:
        model_names = list(MODELS.keys())

    chunks = [
        ("career_history", CHUNK_CAREER, EXPECTED_CAREER),
        ("events_timeline", CHUNK_EVENTS, EXPECTED_EVENTS),
    ]

    all_results = {}

    print("=" * 70)
    print(f"Persistor Extraction Benchmark — {datetime.now().strftime('%Y-%m-%d %H:%M')}")
    print(f"Models: {', '.join(model_names)}")
    print(f"Chunks: {len(chunks)}")
    print("=" * 70)

    for model_name in model_names:
        if model_name not in MODELS:
            print(f"\n⚠️  Unknown model: {model_name}, skipping")
            continue

        config = MODELS[model_name]
        print(f"\n{'─' * 50}")
        print(f"🔬 {model_name} ({config.get('model', '?')})")
        print(f"{'─' * 50}")

        results = run_benchmark(model_name, config, chunks, verbose=args.verbose)
        all_results[model_name] = results

    # ── Summary Table ──
    print(f"\n{'=' * 70}")
    print("SUMMARY")
    print(f"{'=' * 70}")
    print(f"{'Model':<20} {'Chunk':<18} {'Time':>6} {'Ent R':>6} {'Temp%':>6} {'TempR':>6} {'Score':>7}")
    print(f"{'─' * 20} {'─' * 18} {'─' * 6} {'─' * 6} {'─' * 6} {'─' * 6} {'─' * 7}")

    model_averages = []
    for model_name, results in all_results.items():
        scores = []
        for r in results:
            if "error" in r:
                print(f"{model_name:<20} {r['chunk']:<18} {r['elapsed_s']:>5.1f}s {'ERR':>6} {'':>6} {'':>6} {'ERR':>7}")
            else:
                e = r["entities"]
                t = r["temporal"]
                s = r["overall_score"]
                scores.append(s)
                print(f"{model_name:<20} {r['chunk']:<18} {r['elapsed_s']:>5.1f}s {e['recall']:>6.3f} {t['temporal_pct']:>5.1f}% {t['temporal_recall']:>6.3f} {s:>7.3f}")

        if scores:
            avg = sum(scores) / len(scores)
            total_time = sum(r["elapsed_s"] for r in results)
            model_averages.append((model_name, avg, total_time))

    if model_averages:
        print(f"\n{'─' * 70}")
        print(f"{'Model':<20} {'Avg Score':>10} {'Total Time':>12}")
        print(f"{'─' * 20} {'─' * 10} {'─' * 12}")
        for name, avg, total in sorted(model_averages, key=lambda x: -x[1]):
            bar = "█" * int(avg * 20)
            print(f"{name:<20} {avg:>10.3f} {total:>10.1f}s  {bar}")

    # Write JSON output
    if args.output:
        with open(args.output, "w") as f:
            json.dump({
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "results": all_results,
                "summary": [{"model": n, "avg_score": a, "total_time_s": t} for n, a, t in model_averages],
            }, f, indent=2)
        print(f"\n📄 Results written to {args.output}")


if __name__ == "__main__":
    main()
