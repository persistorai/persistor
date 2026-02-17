import { resolveConfig, defaultConfig } from './config.ts';
import { mergeResults } from './result-merger.ts';
import { createUnifiedGetTool } from './unified-get.ts';

import type { PersistorClient } from './persistor-client.ts';
import type { OpenClawTool } from './types.ts';

let passed = 0,
  failed = 0;

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

async function test(name: string, fn: () => void | Promise<void>) {
  try {
    await fn();
    passed++;
    console.log(`  ✅ ${name}`);
  } catch (e: unknown) {
    failed++;
    console.log(`  ❌ ${name}: ${(e as Error).message}`);
  }
}

async function runTests() {
  console.log('\n🧪 memory-persistor tests\n');

  // --- Result Merger ---
  const w = { file: 1.0, persistor: 0.9 };
  const fileR = [{ path: 'MEMORY.md', snippet: 'hello', score: 0.8, line: 1 }];
  const persR = [
    { id: 'a', type: 'concept', label: 'T', properties: {}, salience_score: 75, score: 0.7 },
  ];

  await test('merge: file + persistor in score order', () => {
    const r = mergeResults(fileR, persR, w);
    assert(r.length === 2, `expected 2, got ${r.length}`);
    const first = r[0];
    const second = r[1];
    assert(first !== undefined, 'expected at least one result');
    assert(second !== undefined, 'expected at least two results');
    assert(first.score >= second.score, 'not sorted by score');
    assert(first.source === 'file', 'file should rank first (0.8*1.0 > 0.7*0.9)');
  });

  await test('merge: empty persistor → file only', () => {
    const r = mergeResults(fileR, [], w);
    const first = r[0];
    assert(first !== undefined, 'expected at least one result');
    assert(r.length === 1 && first.source === 'file', 'should have file only');
    assert(first.score === 0.8, `expected 0.8, got ${first.score}`);
  });

  await test('merge: empty file → persistor only', () => {
    const r = mergeResults([], persR, w);
    const first = r[0];
    assert(first !== undefined, 'expected at least one result');
    assert(r.length === 1 && first.source === 'persistor', 'should have persistor only');
  });

  await test('merge: both empty → []', () => {
    assert(mergeResults([], [], w).length === 0, 'should be empty');
  });

  await test('merge: score normalization uses salience_score/100 when no score', () => {
    const noScore = [{ id: 'b', type: 'concept', label: 'X', properties: {}, salience_score: 60 }];
    const r = mergeResults([], noScore, { file: 1, persistor: 1 });
    const first = r[0];
    assert(first !== undefined, 'expected at least one result');
    assert(Math.abs(first.score - 0.6) < 0.001, `expected ~0.6, got ${first.score}`);
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
    process.env.TEST_PERSISTOR_KEY = 'secret123';
    const c = resolveConfig({ persistor: { apiKey: '${TEST_PERSISTOR_KEY}' } });
    assert(c.persistor.apiKey === 'secret123', `expected secret123, got ${c.persistor.apiKey}`);
    delete process.env.TEST_PERSISTOR_KEY;
  });

  // --- Unified Get ---
  const mockFileGet = {
    name: 'memory_get',
    execute: async (_id: string, p: Record<string, unknown>) => `file:${String(p.path)}`,
  };
  const mockClient = {
    getNode: async (id: string) => ({
      id,
      type: 'concept',
      label: 'T',
      properties: {},
      salience: 75,
    }),
    getContext: async () => null,
    checkHealth: async () => true,
  };
  const cfg = resolveConfig({});
  const getTool = createUnifiedGetTool(
    mockFileGet as unknown as OpenClawTool,
    mockClient as unknown as PersistorClient,
    cfg,
  );

  await test('get: file path routes to file tool', async () => {
    const r = await getTool.execute('t1', { path: 'memory/notes.md' });
    assert(r === 'file:memory/notes.md', `unexpected: ${JSON.stringify(r)}`);
  });

  await test('get: UUID routes to persistor', async () => {
    const r = await getTool.execute('t2', { path: '12345678-1234-1234-1234-123456789abc' });
    const text = (r as Record<string, unknown>).content;
    assert(
      Array.isArray(text) && JSON.stringify(text).includes('Node:'),
      `expected node output, got: ${JSON.stringify(r)}`,
    );
  });

  await test('get: non-file non-UUID tries persistor then file', async () => {
    const freshFileGet = {
      name: 'memory_get',
      execute: async (_id: string, p: Record<string, unknown>) => `file:${String(p.path)}`,
    };
    const failClient = {
      ...mockClient,
      getNode: async () => {
        throw new Error('nope');
      },
    };
    const tool = createUnifiedGetTool(
      freshFileGet as unknown as OpenClawTool,
      failClient as unknown as PersistorClient,
      cfg,
    );
    const r = await tool.execute('t3', { path: 'some-label' });
    assert(
      r === ('file:some-label' as unknown),
      `expected file fallback, got: ${JSON.stringify(r)}`,
    );
  });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

void runTests();
