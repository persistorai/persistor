import { PersistorClient } from '@persistorai/sdk';

import { resolveConfig } from './config.ts';
import { logger } from './logger.ts';
import { createUnifiedGetTool } from './unified-get.ts';
import { createUnifiedSearchTool } from './unified-search.ts';

import type { OpenClawPluginApi } from 'openclaw/plugin-sdk';

/** Extract pluginConfig from the API object — known SDK gap: pluginConfig is not on OpenClawPluginApi type yet. */
// TODO(SDK-XXX): Remove cast once pluginConfig is added to OpenClawPluginApi
function getPluginConfig(api: OpenClawPluginApi): Record<string, unknown> {
  const record = api as unknown as Record<string, unknown>;
  if (
    'pluginConfig' in record &&
    typeof record['pluginConfig'] === 'object' &&
    record['pluginConfig'] !== null
  ) {
    return record['pluginConfig'] as Record<string, unknown>;
  }
  return {};
}

const memoryPersistorPlugin = {
  id: 'memory-persistor',
  name: 'Memory (Persistor)',
  description: 'Unified memory search across workspace files and Persistor knowledge graph',
  kind: 'memory' as const,

  register(api: OpenClawPluginApi) {
    const pluginConfig = getPluginConfig(api);
    const config = resolveConfig(pluginConfig);
    const persistorClient = new PersistorClient({
      url: config.persistor.url,
      apiKey: config.persistor.apiKey,
      timeout: config.persistor.timeout,
    });

    // Fire-and-forget health check
    persistorClient
      .ready()
      .then(() => {
        logger.debug('Persistor connected');
      })
      .catch(() => {
        logger.debug('Persistor unreachable — file-only mode');
      });

    api.registerTool(
      (ctx) => {
        const searchOpts: Record<string, unknown> = {};
        if (ctx.config !== undefined) searchOpts['config'] = ctx.config;
        if (ctx.sessionKey !== undefined) searchOpts['agentSessionKey'] = ctx.sessionKey;
        const fileSearchTool = api.runtime.tools.createMemorySearchTool(searchOpts);
        const getOpts: Record<string, unknown> = {};
        if (ctx.config !== undefined) getOpts['config'] = ctx.config;
        if (ctx.sessionKey !== undefined) getOpts['agentSessionKey'] = ctx.sessionKey;
        const fileGetTool = api.runtime.tools.createMemoryGetTool(getOpts);
        if (!fileSearchTool || !fileGetTool) {
          return null;
        }
        return [
          createUnifiedSearchTool(fileSearchTool, persistorClient, config),
          createUnifiedGetTool(fileGetTool, persistorClient, config),
        ];
      },
      { names: ['memory_search', 'memory_get'] },
    );

    api.registerCli(
      ({ program }) => {
        api.runtime.tools.registerMemoryCli(program);

        const kg = program.command('memory-kg').description('Persistor knowledge graph memory');

        kg.command('status')
          .description('Check Persistor health and stats')
          .action(async () => {
            try {
              await persistorClient.ready();
              console.log('[memory-persistor] [OK] Persistor is healthy');
            } catch {
              console.log('[memory-persistor] [WARN] Persistor is unreachable');
            }
          });

        kg.command('search <query>')
          .description('Search Persistor directly')
          .action(async (query: string) => {
            try {
              const results = await persistorClient.search({ q: query });
              if (results.length === 0) {
                console.log('[memory-persistor] No results.');
                return;
              }
              for (const r of results) {
                console.log(`[${r.type}] ${r.label} (salience: ${r.salience_score}) — ${r.id}`);
              }
            } catch (e: unknown) {
              console.log(`[memory-persistor] Search failed: ${e instanceof Error ? e.message : String(e)}`);
            }
          });
      },
      { commands: ['memory', 'memory-kg'] },
    );
  },
};

export default memoryPersistorPlugin;
