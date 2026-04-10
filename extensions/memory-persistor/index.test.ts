import { resolveConfig, defaultConfig } from './config.ts';
import { mergeResults } from './result-merger.ts';
import { buildRetrievalContext } from './session-context.ts';
import { createUnifiedGetTool } from './unified-get.ts';
import { createUnifiedSearchTool } from './unified-search.ts';

import type { OpenClawTool } from './types.ts';
import type { PersistorClient } from '@persistorai/sdk';

let passed = 0;
let failed = 0;

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

async function test(name: string, fn: () => void | Promise<void>): Promise<void> {
  try {
    await fn();
    passed++;
    console.log(`  [OK] ${name}`);
  } catch (e: unknown) {
    failed++;
    console.log(`  [FAIL] ${name}: ${e instanceof Error ? e.message : String(e)}`);
  }
}

async function runTests(): Promise<void> {
  console.log('\n[memory-persistor] tests\n');

  // --- Result Merger ---
  const w = { file: 1.0, persistor: 0.9 };
  const fileR = [{ path: 'MEMORY.md', snippet: 'hello', score: 0.8, line: 1 }];
  const persR = [
    { id: 'a', type: 'concept', label: 'T', properties: {}, salience_score: 75, score: 0.7 },
  ];

  await test('merge: file + persistor in score order', () => {
    const r = mergeResults(fileR, persR, w, 'hello');
    assert(r.length === 2, `expected 2, got ${r.length}`);
    const first = r[0];
    const second = r[1];
    if (!first) throw new Error('expected first result');
    if (!second) throw new Error('expected second result');
    assert(first.score >= second.score, 'not sorted by score');
    assert(first.source === 'file', 'file should rank first (0.8*1.0 > 0.7*0.9)');
  });

  await test('merge: empty persistor -> file only', () => {
    const r = mergeResults(fileR, [], w, 'hello');
    const first = r[0];
    if (!first) throw new Error('expected result');
    assert(r.length === 1 && first.source === 'file', 'should have file only');
    assert(first.score === 0.8, `expected 0.8, got ${first.score}`);
  });

  await test('merge: empty file -> persistor only', () => {
    const r = mergeResults([], persR, w, 'hello');
    const first = r[0];
    if (!first) throw new Error('expected result');
    assert(r.length === 1 && first.source === 'persistor', 'should have persistor only');
  });

  await test('merge: both empty -> []', () => {
    assert(mergeResults([], [], w, 'hello').length === 0, 'should be empty');
  });

  await test('merge: score normalization uses salience_score/100 when no score', () => {
    const noScore = [{ id: 'b', type: 'concept', label: 'X', properties: {}, salience_score: 60 }];
    const r = mergeResults([], noScore, { file: 1, persistor: 1 }, 'hello');
    const first = r[0];
    if (!first) throw new Error('expected result');
    assert(Math.abs(first.score - 0.6) < 0.001, `expected ~0.6, got ${first.score}`);
  });

  await test('merge: session entities boost matching graph results', () => {
    const context = buildRetrievalContext('who is brian', {
      currentSessionEntities: ['Brian'],
      recentMessages: ['We were just discussing Brian and DeerPrint'],
    });
    const fileOnly = [{ path: 'memory/notes.md', snippet: 'misc unrelated note', score: 0.72 }];
    const graph = [
      { id: 'brian', type: 'person', label: 'Brian', properties: { org: 'DeerPrint' }, salience_score: 65 },
    ];
    const r = mergeResults(fileOnly, graph, { file: 1, persistor: 1 }, 'who is brian', context);
    const first = r[0];
    if (!first) throw new Error('expected result');
    assert(first.source === 'persistor', 'graph result should outrank unrelated file result');
  });

  await test('session context: infers file preference from active work context', () => {
    const context = buildRetrievalContext('how should I implement the fix', {
      activeWorkContext: ['repo persistor branch fix/session-aware-retrieval'],
    });
    assert(context.sourcePreference === 'file', `expected file, got ${context.sourcePreference}`);
    assert(context.queryVariants.length >= 2, 'expected query expansion with work context');
  });

  // --- Config ---
  await test('config: defaults applied for {}', () => {
    const c = resolveConfig({});
    assert(c.persistor.url === defaultConfig.persistor.url, 'url mismatch');
    assert(c.persistor.timeout === 3000, 'timeout mismatch');
    assert(c.weights.file === 1.0, 'weight mismatch');
  });

  await test('config: custom values override', () => {
    const c = resolveConfig({
      persistor: { url: 'http://x:9999', timeout: 5000 },
      weights: { file: 0.5 },
    });
    assert(c.persistor.url === 'http://x:9999', 'url not overridden');
    assert(c.persistor.timeout === 5000, 'timeout not overridden');
    assert(c.weights.file === 0.5, 'weight not overridden');
  });

  await test('config: env var resolution for apiKey', () => {
    process.env['TEST_PERSISTOR_KEY'] = 'secret123';
    const c = resolveConfig({ persistor: { apiKey: '${TEST_PERSISTOR_KEY}' } });
    assert(c.persistor.apiKey === 'secret123', `expected secret123, got ${c.persistor.apiKey}`);
    delete process.env['TEST_PERSISTOR_KEY'];
  });

  // --- Unified Get ---
  const mockFileGet = {
    name: 'memory_get',
    execute: (
      _id: string,
      p: Record<string, unknown>,
      _signal?: AbortSignal,
      _onUpdate?: unknown,
    ) =>
      Promise.resolve({
        content: [{ type: 'text' as const, text: `file:${String(p['path'])}` }],
        details: undefined,
      }),
  };
  const mockClient = {
    getNode: (id: string) =>
      Promise.resolve({
        id,
        type: 'concept',
        label: 'T',
        properties: {},
        salience_score: 75,
      }),
    getContext: () => Promise.resolve(null),
    checkHealth: () => Promise.resolve(true),
  };
  const cfg = resolveConfig({});
  const getTool = createUnifiedGetTool(
    mockFileGet as unknown as OpenClawTool,
    mockClient as unknown as PersistorClient,
    cfg,
  );

  await test('get: file path routes to file tool', async () => {
    const r = await getTool.execute('t1', { path: 'memory/notes.md' });
    const text = r.content[0];
    assert(
      text?.type === 'text' && text.text === 'file:memory/notes.md',
      `unexpected: ${JSON.stringify(r)}`,
    );
  });

  await test('get: UUID routes to persistor', async () => {
    const r = await getTool.execute('t2', { path: '12345678-1234-1234-1234-123456789abc' });
    assert(
      Array.isArray(r.content) && JSON.stringify(r.content).includes('Node:'),
      `expected node output, got: ${JSON.stringify(r)}`,
    );
  });

  await test('get: non-file non-UUID tries persistor then file', async () => {
    const freshFileGet = {
      name: 'memory_get',
      execute: (
        _id: string,
        p: Record<string, unknown>,
        _signal?: AbortSignal,
        _onUpdate?: unknown,
      ) =>
        Promise.resolve({
          content: [{ type: 'text' as const, text: `file:${String(p['path'])}` }],
          details: undefined,
        }),
    };
    const failClient = {
      ...mockClient,
      getNode: () => Promise.reject(new Error('nope')),
    };
    const tool = createUnifiedGetTool(
      freshFileGet as unknown as OpenClawTool,
      failClient as unknown as PersistorClient,
      cfg,
    );
    const r = await tool.execute('t3', { path: 'some-label' });
    const part = r.content[0];
    assert(
      part?.type === 'text' && part.text === 'file:some-label',
      `expected file fallback, got: ${JSON.stringify(r)}`,
    );
  });

  // --- Unified Search ---
  await test('search: session context expands persistor query and reports meta', async () => {
    const receivedQueries: string[] = [];
    const fileSearchTool = {
      name: 'memory_search',
      label: 'Memory Search',
      description: 'search files',
      parameters: {},
      execute: () =>
        Promise.resolve({
          content: [
            {
              type: 'text' as const,
              text: JSON.stringify({
                results: [{ path: 'memory/tasks.md', snippet: 'session-aware retrieval task', score: 0.7 }],
              }),
            },
          ],
          details: undefined,
        }),
    };
    const searchClient = {
      searchHybrid: (input: { q: string }) => {
        receivedQueries.push(input.q);
        return Promise.resolve([
          {
            id: input.q.includes('Brian') ? 'person-1' : `query:${input.q}`,
            type: 'person',
            label: 'Brian',
            properties: { project: 'Persistor' },
            salience_score: 70,
          },
        ]);
      },
      searchSemantic: () => Promise.resolve([]),
      search: () => Promise.resolve([]),
    };
    const tool = createUnifiedSearchTool(
      fileSearchTool as unknown as OpenClawTool,
      searchClient as unknown as PersistorClient,
      cfg,
    );

    const result = await tool.execute('t4', {
      query: 'who is working on persistor',
      currentSessionEntities: ['Brian'],
      recentMessages: ['Brian is implementing session-aware retrieval'],
      activeWorkContext: ['persistor repo retrieval task'],
    });

    const text = result.content[0];
    if (text?.type !== 'text') throw new Error('expected text result');
    const payload = JSON.parse(text.text) as {
      results: { source: string }[];
      meta: { sourcePreference: string; queryVariants: string[]; currentSessionEntities: string[] };
    };
    assert(receivedQueries.length >= 2, `expected expanded queries, got ${receivedQueries.length}`);
    assert(payload.meta.sourcePreference === 'both', 'expected combined source preference');
    assert(payload.meta.currentSessionEntities[0] === 'Brian', 'expected session entities in meta');
    assert(payload.results.some((entry) => entry.source === 'persistor'), 'expected persistor result');
    assert(payload.results.some((entry) => entry.source === 'file'), 'expected file result');
  });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

void runTests();
