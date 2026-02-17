/** A result from the built-in file memory search */
export interface FileSearchResult {
  path: string;
  snippet: string;
  score: number;
  line?: number;
  [key: string]: unknown;
}

/** A result from Persistor search API */
export interface PersistorSearchResult {
  id: string;
  type: string;
  label: string;
  properties: Record<string, unknown>;
  salience_score: number;
  score?: number;
}

/** Unified result after merging */
export interface UnifiedSearchResult {
  source: 'file' | 'persistor';
  score: number;
  path?: string;
  snippet?: string;
  line?: number;
  nodeId?: string;
  nodeType?: string;
  label?: string;
  properties?: Record<string, unknown>;
  salienceScore?: number;
  raw?: Record<string, unknown>;
}

export interface MergeWeights {
  file: number;
  persistor: number;
}

const clamp01 = (n: number): number => Math.max(0, Math.min(1, n));

export function mergeResults(
  fileResults: FileSearchResult[],
  persistorResults: PersistorSearchResult[],
  weights: MergeWeights,
): UnifiedSearchResult[] {
  const fileUnified: UnifiedSearchResult[] = fileResults.map((r, i) => {
    const { path, snippet, score, line, ...rest } = r;
    return {
      source: 'file' as const,
      score: clamp01(score) * weights.file,
      path,
      snippet,
      line,
      raw: Object.keys(rest).length ? rest : undefined,
    };
  });

  const persistorUnified: UnifiedSearchResult[] = persistorResults.map((r, i) => {
    const normalized = r.score != null ? clamp01(r.score) : clamp01(r.salience_score / 100);
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
