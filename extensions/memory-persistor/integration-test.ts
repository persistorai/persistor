/**
 * Integration test — hits the live Persistor service.
 * Run: npx tsx integration-test.ts
 */
import { PersistorClient } from './persistor-client.ts';

const API_KEY = process.env['MEMORY_SERVICE_API_KEY'] ?? '';
const BASE_URL = process.env['PERSISTOR_URL'] ?? 'http://127.0.0.1:3030';

const client = new PersistorClient({
  url: BASE_URL,
  apiKey: API_KEY,
  timeout: 10_000,
  searchMode: 'hybrid',
  searchLimit: 5,
});

let passed = 0;
let failed = 0;

async function test(name: string, fn: () => Promise<void>): Promise<void> {
  try {
    await fn();
    console.log(`  ✅ ${name}`);
    passed++;
  } catch (e: unknown) {
    console.error(`  ❌ ${name}:`, e);
    failed++;
  }
}

function assert(condition: boolean, msg: string): void {
  if (!condition) throw new Error(`Assertion failed: ${msg}`);
}

console.log('\n[memory-persistor] Integration Tests (live Persistor)\n');

await test('health check returns true', async () => {
  const ok = await client.checkHealth();
  assert(ok, 'checkHealth should return true');
});

await test('search returns results for known entity', async () => {
  const results = await client.search('DeerPrint');
  assert(Array.isArray(results), 'results should be array');
  assert(results.length > 0, `expected results for "DeerPrint", got ${String(results.length)}`);
  const first = results[0];
  assert(typeof first.id === 'string' && first.id.length > 0, 'result should have id');
  assert(typeof first.type === 'string', 'result should have type');
  assert(typeof first.label === 'string', 'result should have label');
  console.log(`    → Found ${String(results.length)} results, top: "${first.label}" (${first.type})`);
});

await test('search with semantic mode works', async () => {
  const results = await client.search('deer identification', { mode: 'semantic', limit: 3 });
  assert(Array.isArray(results), 'results should be array');
  console.log(`    → Semantic search returned ${String(results.length)} results`);
});

await test('search with text mode works', async () => {
  const results = await client.search('Brian', { mode: 'text', limit: 3 });
  assert(Array.isArray(results), 'results should be array');
  console.log(`    → Text search returned ${String(results.length)} results`);
});

await test('search with empty query returns empty', async () => {
  const results = await client.search('');
  assert(Array.isArray(results), 'results should be array');
});

await test('search with long query truncates safely', async () => {
  const longQuery = 'a'.repeat(2000);
  const results = await client.search(longQuery);
  assert(Array.isArray(results), 'should not throw on long query');
});

// Get a real node ID from search to test getNode/getContext
const searchResults = await client.search('DeerPrint');
const testNodeId = searchResults.length > 0 ? searchResults[0].id : null;

if (testNodeId) {
  await test(`getNode returns valid node (${testNodeId})`, async () => {
    const node = await client.getNode(testNodeId);
    assert(node !== null, 'node should not be null');
    assert(node!.id === testNodeId, 'node id should match');
    assert(typeof node!.label === 'string', 'node should have label');
    assert(typeof node!.salience_score === 'number', 'node should have salience_score');
    console.log(`    → Node: "${node!.label}" (salience: ${String(node!.salience_score)})`);
  });

  await test(`getContext returns node + neighbors (${testNodeId})`, async () => {
    const ctx = await client.getContext(testNodeId);
    assert(ctx !== null, 'context should not be null');
    assert(ctx!.node.id === testNodeId, 'context node id should match');
    assert(Array.isArray(ctx!.neighbors), 'should have neighbors array');
    console.log(`    → Context: "${ctx!.node.label}" with ${String(ctx!.neighbors.length)} neighbors`);
  });
} else {
  console.log('  ⚠️  Skipping getNode/getContext — no node ID from search');
}

await test('getNode with fake UUID returns null', async () => {
  const node = await client.getNode('00000000-0000-0000-0000-000000000000');
  assert(node === null, 'fake UUID should return null');
});

await test('constructor rejects non-localhost URL', async () => {
  let threw = false;
  try {
    new PersistorClient({
      url: 'https://evil.example.com',
      apiKey: 'stolen',
      timeout: 5000,
      searchMode: 'hybrid',
      searchLimit: 5,
    });
  } catch {
    threw = true;
  }
  assert(threw, 'should throw on non-localhost URL');
});

console.log(`\n${String(passed)} passed, ${String(failed)} failed\n`);
process.exit(failed > 0 ? 1 : 0);
