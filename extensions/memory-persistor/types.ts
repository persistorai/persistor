/** Tool result returned by OpenClaw tools */
export interface ToolResult {
  content: { type: string; text: string }[];
}

/** Shape of an OpenClaw tool (search, get, etc.) */
export interface OpenClawTool {
  name: string;
  label: string;
  description: string;
  parameters: unknown;
  execute: (toolCallId: string, params: Record<string, unknown>) => Promise<ToolResult>;
}

/** Content part within a tool result */
export interface ToolContentPart {
  type: string;
  text: string;
}

/** Edge in a Persistor context response */
export interface PersistorEdge {
  source: string;
  target: string;
  relation?: string;
  type?: string;
  weight?: number;
}
