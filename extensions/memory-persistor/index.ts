import { resolveConfig } from './config.ts';
import { logger } from './logger.ts';
import { PersistorClient } from './persistor-client.ts';
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
    const persistorClient = new PersistorClient(config.persistor);

    // Fire-and-forget health check
    persistorClient
      .checkHealth()
      .then((ok) => {
        logger.debug(ok ? 'Persistor connected' : 'Persistor unreachable — file-only mode');
      })
      .catch(() => {
        /* health check is fire-and-forget */
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
            const healthy = await persistorClient.checkHealth();
            // Intentional console.log — this is CLI output
            console.log(
              healthy
                ? '[memory-persistor] [OK] Persistor is healthy'
                : '[memory-persistor] [WARN] Persistor is unreachable',
            );
          });

        kg.command('search <query>')
          .description('Search Persistor directly')
          .action(async (query: string) => {
            const results = await persistorClient.search(query);
            // Intentional console.log — CLI output for search results
            if (results.length === 0) {
              console.log('[memory-persistor] No results.');
              return;
            }
            for (const r of results) {
              console.log(`[${r.type}] ${r.label} (salience: ${r.salience_score}) — ${r.id}`);
            }
          });
      },
      { commands: ['memory', 'memory-kg'] },
    );
  },
};

export default memoryPersistorPlugin;
