import { type PersistorEdge, type PersistorSearchResult, type WrappedNeighbor } from './types.ts';

const isRecord = (v: unknown): v is Record<string, unknown> =>
  v != null && typeof v === 'object' && !Array.isArray(v);

function extractArray(body: unknown): unknown[] {
  if (Array.isArray(body)) return body;
  if (isRecord(body)) {
    for (const key of ['nodes', 'results'] as const) {
      const val = body[key];
      if (Array.isArray(val)) return val;
    }
  }
  return [];
}

function isSearchResult(v: unknown): v is PersistorSearchResult {
  return v != null && typeof v === 'object' && 'id' in v && 'type' in v && 'label' in v;
}

function isPersistorNode(v: unknown): v is PersistorNode {
  return (
    v != null &&
    typeof v === 'object' &&
    'id' in v &&
    'type' in v &&
    'label' in v &&
    'salience_score' in v
  );
}

function isPersistorContext(v: unknown): v is PersistorContext {
  return v != null && typeof v === 'object' && 'node' in v && 'neighbors' in v;
}

export interface PersistorNode {
  id: string;
  type: string;
  label: string;
  properties: Record<string, unknown>;
  salience_score: number;
  created_at: string;
  updated_at: string;
}

export interface PersistorContext {
  node: PersistorNode;
  neighbors: (PersistorNode | WrappedNeighbor)[];
  edges?: PersistorEdge[];
}

export interface PersistorClientConfig {
  url: string;
  apiKey: string;
  timeout: number;
  searchMode: 'hybrid' | 'text' | 'semantic';
  searchLimit: number;
}

/** Allowed URL prefixes for sending auth headers */
const ALLOWED_URL_PREFIXES = ['http://localhost', 'http://127.0.0.1', 'http://[::1]'];

function isAllowedUrl(url: string): boolean {
  return ALLOWED_URL_PREFIXES.some((prefix) => url.startsWith(prefix));
}

export class PersistorClient {
  private readonly config: Readonly<PersistorClientConfig>;

  constructor(config: PersistorClientConfig) {
    const cleanUrl = config.url.replace(/\/+$/, '');
    if (!isAllowedUrl(cleanUrl)) {
      throw new Error(
        `[memory-persistor] Refusing non-localhost Persistor URL: ${cleanUrl}. ` +
          `Only localhost URLs are allowed to prevent credential leakage.`,
      );
    }
    this.config = { ...config, url: cleanUrl };
  }

  private headers(): Record<string, string> {
    return {
      Authorization: `Bearer ${this.config.apiKey}`,
      'Content-Type': 'application/json',
    };
  }

  private async request(
    path: string,
    init?: { method?: string; body?: string },
  ): Promise<Response | null> {
    try {
      const res = await fetch(`${this.config.url}${path}`, {
        method: init?.method ?? 'GET',
        headers: this.headers(),
        body: init?.body,
        signal: AbortSignal.timeout(this.config.timeout),
      });
      if (!res.ok) {
        const body = await res.text().catch(() => '');
        console.warn(`[memory-persistor] ${path}: HTTP ${String(res.status)} ${body}`);
        return null;
      }
      return res;
    } catch (e: unknown) {
      console.warn(`[memory-persistor] Persistor ${path}:`, e);
      return null;
    }
  }

  async checkHealth(): Promise<boolean> {
    return (await this.request('/api/v1/ready')) !== null;
  }

  async search(
    query: string,
    opts?: { mode?: string; limit?: number },
  ): Promise<PersistorSearchResult[]> {
    const mode = opts?.mode ?? this.config.searchMode;
    const limit = opts?.limit ?? this.config.searchLimit;
    // Truncate to 500 chars to stay within safe URL/body sizes
    const safeQuery = query.length > 500 ? query.slice(0, 500) : query;
    const segment =
      mode === 'semantic' ? '/search/semantic' : mode === 'text' ? '/search' : '/search/hybrid';
    // GET with query params — Persistor API max is 2000 chars, truncation above keeps us safe
    const params = new URLSearchParams({ q: safeQuery, limit: String(limit) });
    const res = await this.request(`/api/v1${segment}?${params.toString()}`);
    if (!res) return [];
    try {
      const body: unknown = await res.json();
      return extractArray(body).filter(isSearchResult);
    } catch (e: unknown) {
      console.warn('[memory-persistor] Persistor search parse:', e);
      return [];
    }
  }

  async getNode(id: string): Promise<PersistorNode | null> {
    const res = await this.request(`/api/v1/nodes/${encodeURIComponent(id)}`);
    if (!res) return null;
    try {
      const body: unknown = await res.json();
      return isPersistorNode(body) ? body : null;
    } catch (e: unknown) {
      console.warn('[memory-persistor] Persistor getNode parse:', e);
      return null;
    }
  }

  async getContext(id: string): Promise<PersistorContext | null> {
    const res = await this.request(`/api/v1/graph/context/${encodeURIComponent(id)}`);
    if (!res) return null;
    try {
      const body: unknown = await res.json();
      return isPersistorContext(body) ? body : null;
    } catch (e: unknown) {
      console.warn('[memory-persistor] Persistor getContext parse:', e);
      return null;
    }
  }
}
