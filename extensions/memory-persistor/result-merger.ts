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

export function mergeResults(
  fileResults: FileSearchResult[],
  persistorResults: PersistorSearchResult[],
  weights: MergeWeights,
): UnifiedSearchResult[] {
  const fileUnified: UnifiedSearchResult[] = fileResults.map((r) => {
    const { path, snippet, score, line } = r;
    return {
      source: 'file' as const,
      score: clamp01(score) * weights.file,
      path,
      snippet,
      line,
    };
  });

  const persistorUnified: UnifiedSearchResult[] = persistorResults.map((r) => {
    const normalized =
      r.score != null ? clamp01(r.score) : clamp01(r.salience_score / SALIENCE_SCORE_MAX);
    return {
      source: 'persistor' as const,
      score: normalized * weights.persistor,
      nodeId: r.id,
      nodeType: r.type,
      label: r.label,
      properties: r.properties,
      salienceScore: r.salience_score,
      raw: r as unknown as Record<string, unknown>,
    };
  });

  return [...fileUnified, ...persistorUnified].sort((a, b) => b.score - a.score);
}
