import type { PersistorSearchResult, UnifiedSearchResult } from './types.ts';

/** A result from the built-in file memory search */
export interface FileSearchResult {
  path: string;
  snippet: string;
  score: number;
  line?: number | undefined;
  metadata?: Record<string, unknown>;
}

export interface MergeWeights {
  file: number;
  persistor: number;
}

/** Upper bound for salience scores used to normalize to 0–1 range */
const SALIENCE_SCORE_MAX = 100;

const clamp01 = (n: number): number => Math.max(0, Math.min(1, n));

function detectQueryIntent(query: string): 'general' | 'entity' | 'temporal' | 'procedural' {
  const q = query.toLowerCase().trim();
  if (q.includes('what happened') || q.includes('when ') || q.includes('history')) return 'temporal';
  if (q.includes('how ') || q.includes('stance') || q.includes('policy') || q.includes('rule')) return 'procedural';
  if (q.includes('who is') || q.includes('what is') || q.includes('tell me about')) return 'entity';
  return 'general';
}

function normalizeText(text: string): string {
  return text.toLowerCase().replace(/\s+/g, ' ').trim();
}

function dedupeResults(results: UnifiedSearchResult[]): UnifiedSearchResult[] {
  const seen = new Set<string>();
  const deduped: UnifiedSearchResult[] = [];
  for (const result of results) {
    const key =
      result.source === 'file'
        ? `file:${result.path ?? ''}:${normalizeText(result.snippet ?? '')}`
        : `persistor:${result.nodeId ?? ''}`;
    if (seen.has(key)) continue;
    seen.add(key);
    deduped.push(result);
  }
  return deduped;
}

function scoreFileResult(result: FileSearchResult, weights: MergeWeights, intent: string): number {
  let score = clamp01(result.score) * weights.file;
  const path = result.path.toLowerCase();
  if (intent === 'procedural' && (path.includes('memory') || path.includes('runbook') || path.includes('agents'))) {
    score += 0.08;
  }
  if (intent === 'temporal' && /\b\d{4}-\d{2}-\d{2}\b/.test(result.snippet)) {
    score += 0.05;
  }
  return clamp01(score);
}

function scorePersistorResult(result: PersistorSearchResult, weights: MergeWeights, intent: string): number {
  const normalized =
    result.score != null ? clamp01(result.score) : clamp01(result.salience_score / SALIENCE_SCORE_MAX);
  let score = normalized * weights.persistor;
  if (intent === 'entity') {
    score += 0.05;
  }
  if (intent === 'temporal' && ('date' in result.properties || 'year' in result.properties)) {
    score += 0.05;
  }
  return clamp01(score);
}

export function mergeResults(
  fileResults: FileSearchResult[],
  persistorResults: PersistorSearchResult[],
  weights: MergeWeights,
  query = '',
): UnifiedSearchResult[] {
  const intent = detectQueryIntent(query);

  const fileUnified: UnifiedSearchResult[] = fileResults.map((r) => {
    const { path, snippet, line } = r;
    return {
      source: 'file' as const,
      score: scoreFileResult(r, weights, intent),
      path,
      snippet,
      line,
    };
  });

  const persistorUnified: UnifiedSearchResult[] = persistorResults.map((r) => {
    return {
      source: 'persistor' as const,
      score: scorePersistorResult(r, weights, intent),
      nodeId: r.id,
      nodeType: r.type,
      label: r.label,
      properties: r.properties,
      salienceScore: r.salience_score,
      raw: {
        id: r.id,
        type: r.type,
        label: r.label,
        properties: r.properties,
        salience_score: r.salience_score,
        score: r.score,
      },
    };
  });

  return dedupeResults([...fileUnified, ...persistorUnified]).sort((a, b) => b.score - a.score);
}
