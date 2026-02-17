import { resolveConfig, defaultConfig } from './config.ts';
import { mergeResults } from './result-merger.ts';
import { createUnifiedGetTool } from './unified-get.ts';

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
  } catch (e: any) {
    failed++;
    console.log(`  ❌ ${name}: ${e.message}`);
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
    assert(r[0].score >= r[1].score, 'not sorted by score');
    assert(r[0].source === 'file', 'file should rank first (0.8*1.0 > 0.7*0.9)');
  });

  await test('merge: empty persistor → file only', () => {
    const r = mergeResults(fileR, [], w);
    assert(r.length === 1 && r[0].source === 'file', 'should have file only');
    assert(r[0].score === 0.8, `expected 0.8, got ${r[0].score}`);
  });

  await test('merge: empty file → persistor only', () => {
    const r = mergeResults([], persR, w);
    assert(r.length === 1 && r[0].source === 'persistor', 'should have persistor only');
  });

  await test('merge: both empty → []', () => {
    assert(mergeResults([], [], w).length === 0, 'should be empty');
  });

  await test('merge: score normalization uses salience_score/100 when no score', () => {
    const noScore = [{ id: 'b', type: 'concept', label: 'X', properties: {}, salience_score: 60 }];
    const r = mergeResults([], noScore, { file: 1, persistor: 1 });
    assert(Math.abs(r[0].score - 0.6) < 0.001, `expected ~0.6, got ${r[0].score}`);
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
    execute: async (_id: string, p: any) => `file:${p.path}`,
  };
  const mockClient: any = {
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
  const getTool = createUnifiedGetTool(mockFileGet, mockClient, cfg);

  await test('get: file path routes to file tool', async () => {
    const r = await getTool.execute('t1', { path: 'memory/notes.md' });
    assert(r === 'file:memory/notes.md', `unexpected: ${r}`);
  });

  await test('get: UUID routes to persistor', async () => {
    const r = await getTool.execute('t2', { path: '12345678-1234-1234-1234-123456789abc' });
    assert(typeof r === 'string' && r.includes('Node:'), `expected node output, got: ${r}`);
  });

  await test('get: non-file non-UUID tries persistor then file', async () => {
    const failClient: any = {
      ...mockClient,
      getNode: async () => {
        throw new Error('nope');
      },
    };
    const tool = createUnifiedGetTool(mockFileGet, failClient, cfg);
    const r = await tool.execute('t3', { path: 'some-label' });
    assert(r === 'file:some-label', `expected file fallback, got: ${r}`);
  });

  console.log(`\n${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

void runTests();
