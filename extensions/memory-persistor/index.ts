import { resolveConfig } from './config.ts';
import { PersistorClient } from './persistor-client.ts';
import { createUnifiedGetTool } from './unified-get.ts';
import { createUnifiedSearchTool } from './unified-search.ts';

import type { OpenClawPluginApi } from 'openclaw/plugin-sdk';

const memoryPersistorPlugin = {
  id: 'memory-persistor',
  name: 'Memory (Persistor)',
  description: 'Unified memory search across workspace files and Persistor knowledge graph',
  kind: 'memory' as const,

  register(api: OpenClawPluginApi) {
    const pluginConfig = (api as Record<string, unknown>)['pluginConfig'] as
      | Record<string, unknown>
      | undefined;
    const config = resolveConfig(pluginConfig ?? {});
    const persistorClient = new PersistorClient(config.persistor);

    // Fire-and-forget health check
    void persistorClient.checkHealth().then((ok) => {
      console.log(
        ok
          ? '[memory-persistor] ✅ Persistor connected'
          : '[memory-persistor] ⚠️ Persistor unreachable — file-only mode',
      );
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
            console.log(healthy ? '✅ Persistor is healthy' : '❌ Persistor is unreachable');
          });

        kg.command('search <query>')
          .description('Search Persistor directly')
          .action(async (query: string) => {
            const results = await persistorClient.search(query);
            if (results.length === 0) {
              console.log('No results.');
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
