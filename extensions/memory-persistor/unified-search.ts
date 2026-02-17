import { mergeResults } from './result-merger.ts';

import type { PersistorPluginConfig } from './config.ts';
import type { PersistorClient } from './persistor-client.ts';
import type { FileSearchResult } from './result-merger.ts';
import type { OpenClawTool, PersistorSearchResult, ToolContentPart, ToolResult } from './types.ts';
import type { TextContent } from '@mariozechner/pi-ai';

/**
 * Extract the JSON payload from a tool result.
 * Tools return { content: [{ type: "text", text: JSON.stringify(payload) }] }
 */
function extractToolPayload(result: unknown): unknown {
  if (!result || typeof result !== 'object') return null;
  if (!('content' in result)) return null;
  const content = (result as { content: unknown }).content;
  if (Array.isArray(content)) {
    const isToolContentPart = (v: unknown): v is ToolContentPart =>
      v != null && typeof v === 'object' && 'type' in v && typeof (v as Record<string, unknown>)['type'] === 'string';
    const parts = content.filter(isToolContentPart);
    const textPart = parts.find((c): c is TextContent => c.type === 'text');
    if (textPart?.text != null) {
      try {
        return JSON.parse(textPart.text) as unknown;
      } catch {
        return null;
      }
    }
  }
  return null;
}

/** Default score assigned when a file result has no explicit score */
const DEFAULT_UNKNOWN_SCORE = 0.5;

const isRecord = (v: unknown): v is Record<string, unknown> =>
  v != null && typeof v === 'object' && !Array.isArray(v);

function extractFileResults(toolResult: unknown): FileSearchResult[] {
  const payload = extractToolPayload(toolResult);
  if (!isRecord(payload)) return [];
  const obj = payload;
  const results = Array.isArray(obj['results']) ? obj['results'] : [];
  return results.filter(isRecord).map((r) => ({
    path: String(r['path'] ?? r['file'] ?? 'unknown'),
    snippet: String(r['snippet'] ?? r['text'] ?? r['content'] ?? String(r)),
    score: typeof r['score'] === 'number' ? r['score'] : DEFAULT_UNKNOWN_SCORE,
    line:
      typeof r['line'] === 'number'
        ? r['line']
        : typeof r['startLine'] === 'number'
          ? r['startLine']
          : undefined,
  }));
}

function jsonResult(payload: unknown): ToolResult {
  return {
    content: [{ type: 'text', text: JSON.stringify(payload, null, 2) }],
    details: undefined,
  };
}

/**
 * Wraps the built-in file search tool, adding Persistor results.
 * Returns a cloned tool object that preserves all properties (including
 * non-enumerable ones) with only `execute` and `description` overridden.
 */
export function createUnifiedSearchTool(
  fileSearchTool: OpenClawTool,
  persistorClient: PersistorClient,
  config: PersistorPluginConfig,
): OpenClawTool {
  // bind is a no-op for arrow fns but kept for safety if execute is ever a method
  const originalExecute = fileSearchTool.execute.bind(fileSearchTool);

  // Object.create clone preserves prototype chain + own property descriptors.
  // Assumes no private class fields (WeakMap-based or #-private) on the tool.
  const wrappedTool = Object.create(
    Object.getPrototypeOf(fileSearchTool) as object,
    Object.getOwnPropertyDescriptors(fileSearchTool),
  ) as OpenClawTool;

  wrappedTool.description =
    'Semantically search MEMORY.md + memory/*.md files AND the Persistor knowledge graph. Returns unified results from both sources.';

  wrappedTool.execute = async (
    toolCallId: string,
    params: Record<string, unknown>,
    _signal?: AbortSignal,
    _onUpdate?: (partialResult: ToolResult) => void,
  ): Promise<ToolResult> => {
    const query = typeof params['query'] === 'string' ? params['query'] : '';
    const maxResults = typeof params['maxResults'] === 'number' ? params['maxResults'] : 20;
    const minScore = typeof params['minScore'] === 'number' ? params['minScore'] : 0;

    const [fileResult, persistorResult] = await Promise.allSettled([
      originalExecute(toolCallId, params),
      persistorClient.search(query, { limit: config.persistor.searchLimit }),
    ]);

    const fileResults =
      fileResult.status === 'fulfilled' ? extractFileResults(fileResult.value) : [];
    let persistorResults: PersistorSearchResult[] = [];
    let persistorAvailable = true;
    if (persistorResult.status === 'fulfilled') {
      persistorResults = persistorResult.value;
    } else {
      persistorAvailable = false;
    }

    const merged = mergeResults(fileResults, persistorResults, config.weights);
    const filtered = merged.filter((r) => r.score >= minScore).slice(0, maxResults);

    return jsonResult({
      results: filtered.map((r) => {
        if (r.source === 'file') {
          return {
            source: 'file',
            path: r.path,
            snippet: r.snippet,
            score: r.score,
            line: r.line,
          };
        }
        return {
          source: 'persistor',
          nodeId: r.nodeId,
          nodeType: r.nodeType,
          label: r.label,
          properties: r.properties,
          salienceScore: r.salienceScore,
          score: r.score,
        };
      }),
      meta: {
        persistorAvailable,
        totalFile: fileResults.length,
        totalPersistor: persistorResults.length,
      },
    });
  };

  return wrappedTool;
}
