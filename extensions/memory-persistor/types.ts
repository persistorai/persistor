import type { TextContent, ImageContent } from '@mariozechner/pi-ai';

/** Content part within a tool result — re-exported from pi-ai for SDK compatibility. */
export type ToolContentPart = TextContent | ImageContent;

/** Tool result returned by OpenClaw tools */
export interface ToolResult {
  content: ToolContentPart[];
  details: unknown;
}

/** Shape of an OpenClaw tool (search, get, etc.) */
export interface OpenClawTool {
  name: string;
  label: string;
  description: string;
  parameters: Record<string, unknown>;
  execute: (
    toolCallId: string,
    params: Record<string, unknown>,
    signal?: AbortSignal,
    onUpdate?: (partialResult: ToolResult) => void,
  ) => Promise<ToolResult>;
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

/** Edge in a Persistor context response */
export interface PersistorEdge {
  source: string;
  target: string;
  relation?: string;
  type?: string;
  weight?: number;
}

/** A neighbor entry that wraps node + edge + direction */
export interface WrappedNeighbor {
  node: {
    id: string;
    type: string;
    label: string;
    properties: Record<string, unknown>;
    salience_score: number;
  };
  edge: PersistorEdge;
  direction: string;
}

export function isWrappedNeighbor(v: unknown): v is WrappedNeighbor {
  return v != null && typeof v === 'object' && 'node' in v && 'edge' in v;
}

/** Unified result after merging */
export interface UnifiedSearchResult {
  source: 'file' | 'persistor';
  score: number;
  path?: string;
  snippet?: string;
  line?: number | undefined;
  nodeId?: string;
  nodeType?: string;
  label?: string;
  properties?: Record<string, unknown>;
  salienceScore?: number;
  /** Raw Persistor response data — kept for debugging; consider removing if payload size matters. */
  raw?: Record<string, unknown> | undefined;
}
