/** Content part within a tool result */
export interface ToolContentPart {
  type: 'text' | 'image';
  text?: string;
  source?: unknown;
}

/** Tool result returned by OpenClaw tools */
export interface ToolResult {
  content: ToolContentPart[];
  details?: unknown;
}

/** Shape of an OpenClaw tool (search, get, etc.) */
export interface OpenClawTool {
  name: string;
  label: string;
  description: string;
  parameters: unknown;
  execute: (toolCallId: string, params: Record<string, unknown>) => Promise<ToolResult>;
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
  raw?: Record<string, unknown> | undefined;
}
