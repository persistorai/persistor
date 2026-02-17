import { type PersistorEdge, type PersistorSearchResult } from './types.ts';

function extractArray(body: unknown): unknown[] {
  if (Array.isArray(body)) return body;
  if (body != null && typeof body === 'object') {
    for (const key of ['nodes', 'results'] as const) {
      const val = (body as Record<string, unknown>)[key];
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
  neighbors: (PersistorNode | { node: PersistorNode; edge: PersistorEdge; direction: string })[];
  edges?: PersistorEdge[];
}

export interface PersistorClientConfig {
  url: string;
  apiKey: string;
  timeout: number;
  searchMode: 'hybrid' | 'text' | 'semantic';
  searchLimit: number;
}

export class PersistorClient {
  private readonly config: Readonly<PersistorClientConfig>;

  constructor(config: PersistorClientConfig) {
    this.config = { ...config, url: config.url.replace(/\/+$/, '') };
  }

  private headers(): Record<string, string> {
    return {
      Authorization: `Bearer ${this.config.apiKey}`,
      'Content-Type': 'application/json',
    };
  }

  private async request(path: string): Promise<Response | null> {
    try {
      const res = await fetch(`${this.config.url}${path}`, {
        headers: this.headers(),
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
    const safeQuery = query.length > 1500 ? query.slice(0, 1500) : query;
    const segment =
      mode === 'semantic' ? '/search/semantic' : mode === 'text' ? '/search' : '/search/hybrid';
    const res = await this.request(
      `/api/v1${segment}?q=${encodeURIComponent(safeQuery)}&limit=${limit}`,
    );
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
