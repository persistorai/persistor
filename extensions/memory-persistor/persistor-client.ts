import { type PersistorEdge } from './types.ts';

export interface PersistorNode {
  id: string;
  type: string;
  label: string;
  properties: Record<string, unknown>;
  salience_score: number;
  created_at: string;
  updated_at: string;
}

export interface PersistorSearchResult {
  id: string;
  type: string;
  label: string;
  properties: Record<string, unknown>;
  salience_score: number;
  score?: number;
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

  private async get(path: string): Promise<Response | null> {
    try {
      const res = await fetch(`${this.config.url}${path}`, {
        headers: this.headers(),
        signal: AbortSignal.timeout(this.config.timeout),
      });
      if (!res.ok) {
        console.warn(`Persistor ${path}: HTTP ${res.status}`);
        return null;
      }
      return res;
    } catch (e: unknown) {
      console.warn(`Persistor ${path}:`, e);
      return null;
    }
  }

  async checkHealth(): Promise<boolean> {
    return (await this.get('/api/v1/ready')) !== null;
  }

  async search(
    query: string,
    opts?: { mode?: string; limit?: number },
  ): Promise<PersistorSearchResult[]> {
    const mode = opts?.mode ?? this.config.searchMode;
    const limit = opts?.limit ?? this.config.searchLimit;
    const segment =
      mode === 'semantic' ? '/search/semantic' : mode === 'text' ? '/search' : '/search/hybrid';
    const res = await this.get(`/api/v1${segment}?q=${encodeURIComponent(query)}&limit=${limit}`);
    if (!res) return [];
    try {
      const body: unknown = await res.json();
      const obj = body != null && typeof body === 'object' ? (body as Record<string, unknown>) : {};
      const nodes: unknown[] = Array.isArray(body)
        ? (body as unknown[])
        : Array.isArray(obj['nodes'])
          ? (obj['nodes'] as unknown[])
          : Array.isArray(obj['results'])
            ? (obj['results'] as unknown[])
            : [];
      return nodes as PersistorSearchResult[];
    } catch (e: unknown) {
      console.warn('Persistor search parse:', e);
      return [];
    }
  }

  async getNode(id: string): Promise<PersistorNode | null> {
    const res = await this.get(`/api/v1/nodes/${encodeURIComponent(id)}`);
    if (!res) return null;
    try {
      return (await res.json()) as PersistorNode;
    } catch (e: unknown) {
      console.warn('Persistor getNode parse:', e);
      return null;
    }
  }

  async getContext(id: string): Promise<PersistorContext | null> {
    const res = await this.get(`/api/v1/graph/context/${encodeURIComponent(id)}`);
    if (!res) return null;
    try {
      return (await res.json()) as PersistorContext;
    } catch (e: unknown) {
      console.warn('Persistor getContext parse:', e);
      return null;
    }
  }
}
