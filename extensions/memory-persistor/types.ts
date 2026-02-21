import type { TextContent, ImageContent } from '@mariozechner/pi-ai';

// Re-export SDK types for backward compatibility
export type {
  PersistorNode,
  PersistorEdge,
  PersistorSearchResult,
  PersistorContext,
} from '@persistorai/sdk';

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
