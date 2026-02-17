import type { PersistorPluginConfig } from './config.ts';
import type { PersistorClient, PersistorNode, PersistorContext } from './persistor-client.ts';
import type { OpenClawTool, ToolResult } from './types.ts';

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function isFilePath(path: string): boolean {
  return path.includes('/') || path.endsWith('.md');
}

function isUUID(str: string): boolean {
  return UUID_RE.test(str);
}

function jsonWrap(text: string): ToolResult {
  return { content: [{ type: 'text', text }] };
}

function formatNode(node: PersistorNode, context?: PersistorContext | null): string {
  const lines: string[] = [
    `📦 Node: ${node.label} (${node.type})`,
    `ID: ${node.id}`,
    `Salience: ${String(node.salience_score)}`,
  ];
  if (Object.keys(node.properties).length > 0) {
    lines.push('', 'Properties:');
    for (const [k, v] of Object.entries(node.properties)) {
      lines.push(`  ${k}: ${typeof v === 'object' ? JSON.stringify(v) : String(v)}`);
    }
  }
  if (context?.neighbors.length) {
    lines.push('', `Neighbors (${context.neighbors.length}):`);
    for (const n of context.neighbors) {
      const isWrapped = 'node' in n && 'edge' in n;
      const innerNode: PersistorNode = isWrapped ? (n as { node: PersistorNode }).node : n;
      lines.push(`  → ${innerNode.label} (${innerNode.type})`);
    }
  }
  if (context?.edges?.length) {
    lines.push('', 'Edges:');
    for (const e of context.edges) {
      lines.push(`  ${e.source} —[${e.relation ?? e.type ?? '?'}]→ ${e.target}`);
    }
  }
  return lines.join('\n');
}

async function getPersistorNode(
  client: PersistorClient,
  id: string,
  includeContext: boolean,
): Promise<string | null> {
  try {
    const node = await client.getNode(id);
    if (!node) return null;
    const context = includeContext ? await client.getContext(id) : null;
    return formatNode(node, context);
  } catch (e) {
    console.warn(`[memory-persistor] getPersistorNode error:`, e);
    return null;
  }
}

/**
 * Wraps the built-in file get tool, adding Persistor node lookup.
 * Mutates the original tool to preserve all properties the runtime expects.
 */
export function createUnifiedGetTool(
  fileGetTool: OpenClawTool,
  persistorClient: PersistorClient,
  config: PersistorPluginConfig,
): OpenClawTool {
  const originalExecute = fileGetTool.execute.bind(fileGetTool);

  fileGetTool.description =
    'Read from MEMORY.md, memory/*.md files, or Persistor knowledge graph nodes. Pass a file path for files, or a node UUID for Persistor lookups.';

  fileGetTool.execute = async (
    toolCallId: string,
    params: Record<string, unknown>,
  ): Promise<ToolResult> => {
    const path = typeof params['path'] === 'string' ? params['path'] : '';

    // File paths always go to the file tool
    if (isFilePath(path)) {
      return originalExecute(toolCallId, params);
    }

    // UUIDs try Persistor first, then fall back to file tool
    if (isUUID(path)) {
      const result = await getPersistorNode(persistorClient, path, config.persistorContextOnGet);
      if (result) return jsonWrap(result);
      return originalExecute(toolCallId, params);
    }

    // Other strings (slugs/labels) — try Persistor, then file tool
    const result = await getPersistorNode(persistorClient, path, config.persistorContextOnGet);
    if (result) return jsonWrap(result);
    return originalExecute(toolCallId, params);
  };

  return fileGetTool;
}
