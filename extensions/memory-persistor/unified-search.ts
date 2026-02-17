import { mergeResults } from './result-merger.ts';

import type { PersistorPluginConfig } from './config.ts';
import type { PersistorClient, PersistorSearchResult } from './persistor-client.ts';
import type { FileSearchResult } from './result-merger.ts';
import type { OpenClawTool, ToolContentPart } from './types.ts';

/**
 * Extract the JSON payload from a tool result.
 * Tools return { content: [{ type: "text", text: JSON.stringify(payload) }] }
 */
function extractToolPayload(result: unknown): unknown {
  if (!result || typeof result !== 'object') return null;
  const obj = result as Record<string, unknown>;
  if (Array.isArray(obj.content)) {
    const textPart = obj.content.find(
      (c: ToolContentPart) => c?.type === 'text' && typeof c?.text === 'string',
    );
    if (textPart) {
      try {
        return JSON.parse(textPart.text);
      } catch {
        return null;
      }
    }
  }
  return null;
}

function extractFileResults(toolResult: unknown): FileSearchResult[] {
  const payload = extractToolPayload(toolResult);
  if (!payload || typeof payload !== 'object') return [];
  const obj = payload as Record<string, unknown>;
  const results = Array.isArray(obj.results) ? obj.results : [];
  return results.map((r: Record<string, unknown>) => ({
    path: String(r.path ?? r.file ?? 'unknown'),
    snippet: String(r.snippet ?? r.text ?? r.content ?? String(r)),
    score: typeof r.score === 'number' ? r.score : 0.5,
    line:
      typeof r.line === 'number'
        ? r.line
        : typeof r.startLine === 'number'
          ? r.startLine
          : undefined,
  }));
}

function jsonResult(payload: unknown) {
  return { content: [{ type: 'text', text: JSON.stringify(payload, null, 2) }] };
}

/**
 * Wraps the built-in file search tool, adding Persistor results.
 * Returns the SAME tool object with only `execute` and `description` overridden
 * to preserve all other properties the runtime expects.
 */
export function createUnifiedSearchTool(
  fileSearchTool: OpenClawTool,
  persistorClient: PersistorClient,
  config: PersistorPluginConfig,
) {
  const originalExecute = fileSearchTool.execute.bind(fileSearchTool);

  fileSearchTool.description =
    'Semantically search MEMORY.md + memory/*.md files AND the Persistor knowledge graph. Returns unified results from both sources.';

  fileSearchTool.execute = async (
    toolCallId: string,
    params: { query: string; maxResults?: number; minScore?: number },
  ) => {
    const { query, maxResults = 20, minScore = 0 } = params;

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
          return { source: 'file', path: r.path, snippet: r.snippet, score: r.score, line: r.line };
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

  return fileSearchTool;
}
