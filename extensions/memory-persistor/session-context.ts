export type RetrievalSourcePreference = 'file' | 'persistor' | 'both';

export interface RetrievalContext {
  currentSessionEntities: string[];
  recentMessages: string[];
  activeWorkContext: string[];
  sourcePreference: RetrievalSourcePreference;
  queryVariants: string[];
}

const MAX_ENTITY_COUNT = 6;
const MAX_MESSAGE_COUNT = 4;
const MAX_WORK_CONTEXT_COUNT = 4;
const MAX_TEXT_LENGTH = 240;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value != null && typeof value === 'object' && !Array.isArray(value);

function normalizeWhitespace(value: string): string {
  return value.replace(/\s+/g, ' ').trim();
}

function toText(value: unknown): string | null {
  if (typeof value === 'string') {
    const normalized = normalizeWhitespace(value);
    return normalized ? normalized.slice(0, MAX_TEXT_LENGTH) : null;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  if (Array.isArray(value)) {
    const joined = value
      .map((item) => toText(item))
      .filter((item): item is string => item != null)
      .join(' ');
    return joined ? joined.slice(0, MAX_TEXT_LENGTH) : null;
  }
  if (isRecord(value)) {
    const preferredKeys = ['name', 'label', 'title', 'text', 'content', 'message', 'path', 'file'];
    for (const key of preferredKeys) {
      const text = toText(value[key]);
      if (text) return text;
    }
  }
  return null;
}

function toTextList(value: unknown, maxItems: number): string[] {
  if (value == null) return [];
  const items = Array.isArray(value) ? value : [value];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of items) {
    const text = toText(item);
    if (!text) continue;
    const normalized = text.toLowerCase();
    if (seen.has(normalized)) continue;
    seen.add(normalized);
    out.push(text);
    if (out.length >= maxItems) break;
  }
  return out;
}

function pickFirstList(params: Record<string, unknown>, keys: string[], maxItems: number): string[] {
  for (const key of keys) {
    const list = toTextList(params[key], maxItems);
    if (list.length > 0) return list;
  }
  return [];
}

function detectSourcePreference(
  query: string,
  currentSessionEntities: string[],
  activeWorkContext: string[],
): RetrievalSourcePreference {
  const q = query.toLowerCase();
  const joinedWork = activeWorkContext.join(' ').toLowerCase();
  const fileSignals =
    /(file|files|repo|repository|code|implement|fix|task|todo|worktree|path|folder|docs?|runbook|memory\.md)/.test(q) ||
    /(file|repo|code|task|branch|path|folder|docs?)/.test(joinedWork);
  const graphSignals =
    /(who is|what is|relationship|related|entity|person|org|project|history|timeline)/.test(q) ||
    currentSessionEntities.length > 0;

  if (fileSignals && !graphSignals) return 'file';
  if (graphSignals && !fileSignals) return 'persistor';
  return 'both';
}

function buildQueryVariants(
  query: string,
  currentSessionEntities: string[],
  recentMessages: string[],
  activeWorkContext: string[],
): string[] {
  const variants = [query];
  if (currentSessionEntities.length > 0) {
    variants.push(`${query} ${currentSessionEntities.slice(0, 3).join(' ')}`.trim());
  }
  if (activeWorkContext.length > 0) {
    variants.push(`${query} ${activeWorkContext[0]}`.trim());
  }
  if (recentMessages.length > 0) {
    variants.push(`${query} ${recentMessages[0]}`.trim());
  }

  const seen = new Set<string>();
  return variants
    .map((value) => normalizeWhitespace(value).slice(0, 500))
    .filter((value) => value.length > 0)
    .filter((value) => {
      const normalized = value.toLowerCase();
      if (seen.has(normalized)) return false;
      seen.add(normalized);
      return true;
    })
    .slice(0, 4);
}

export function buildRetrievalContext(query: string, params: Record<string, unknown>): RetrievalContext {
  const currentSessionEntities = pickFirstList(
    params,
    ['currentSessionEntities', 'sessionEntities', 'entities'],
    MAX_ENTITY_COUNT,
  );
  const recentMessages = pickFirstList(
    params,
    ['recentMessages', 'messages', 'recentTurns'],
    MAX_MESSAGE_COUNT,
  );
  const activeWorkContext = pickFirstList(
    params,
    ['activeWorkContext', 'workContext', 'activeTask', 'taskContext'],
    MAX_WORK_CONTEXT_COUNT,
  );

  return {
    currentSessionEntities,
    recentMessages,
    activeWorkContext,
    sourcePreference: detectSourcePreference(query, currentSessionEntities, activeWorkContext),
    queryVariants: buildQueryVariants(query, currentSessionEntities, recentMessages, activeWorkContext),
  };
}
