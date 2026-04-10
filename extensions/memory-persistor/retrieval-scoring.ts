import type { FileSearchResult } from './result-merger.ts';
import type { RetrievalContext, RetrievalSourcePreference } from './session-context.ts';
import type { PersistorSearchResult } from '@persistorai/sdk';

const clamp01 = (value: number): number => Math.max(0, Math.min(1, value));

function normalize(value: string): string {
  return value.toLowerCase();
}

function containsAny(haystack: string, needles: string[]): boolean {
  const normalizedHaystack = normalize(haystack);
  return needles.some((needle) => normalizedHaystack.includes(normalize(needle)));
}

function sourceMultiplier(preference: RetrievalSourcePreference, source: 'file' | 'persistor'): number {
  if (preference === 'both') return 1;
  if (preference === source) return 1.12;
  return 0.92;
}

export function scoreFileContextBoost(result: FileSearchResult, context: RetrievalContext): number {
  const pathAndSnippet = `${result.path} ${result.snippet}`;
  let boost = 0;

  if (context.activeWorkContext.length > 0 && containsAny(pathAndSnippet, context.activeWorkContext)) {
    boost += 0.08;
  }
  if (context.recentMessages.length > 0 && containsAny(pathAndSnippet, context.recentMessages)) {
    boost += 0.05;
  }
  if (context.currentSessionEntities.length > 0 && containsAny(pathAndSnippet, context.currentSessionEntities)) {
    boost += 0.04;
  }

  const weighted = result.score * sourceMultiplier(context.sourcePreference, 'file');
  return clamp01(weighted + boost) - clamp01(result.score);
}

export function scorePersistorContextBoost(
  result: PersistorSearchResult,
  context: RetrievalContext,
): number {
  const searchable = `${result.label} ${JSON.stringify(result.properties)}`;
  const baseScore = result.score ?? result.salience_score / 100;
  let boost = 0;

  if (context.currentSessionEntities.length > 0 && containsAny(searchable, context.currentSessionEntities)) {
    boost += 0.1;
  }
  if (context.recentMessages.length > 0 && containsAny(searchable, context.recentMessages)) {
    boost += 0.05;
  }
  if (context.activeWorkContext.length > 0 && containsAny(searchable, context.activeWorkContext)) {
    boost += 0.04;
  }

  const weighted = baseScore * sourceMultiplier(context.sourcePreference, 'persistor');
  return clamp01(weighted + boost) - clamp01(baseScore);
}
