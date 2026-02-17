/** Configuration for the memory-persistor plugin */
export interface PersistorPluginConfig {
  persistor: {
    url: string;
    apiKey: string;
    timeout: number;
    searchMode: 'hybrid' | 'text' | 'semantic';
    searchLimit: number;
  };
  weights: {
    file: number;
    persistor: number;
  };
  persistorContextOnGet: boolean;
}

/** Default configuration values */
export const defaultConfig: PersistorPluginConfig = {
  persistor: {
    url: 'http://localhost:3030',
    apiKey: '',
    timeout: 3000,
    searchMode: 'hybrid',
    searchLimit: 10,
  },
  weights: {
    file: 1.0,
    persistor: 0.9,
  },
  persistorContextOnGet: true,
};

interface RawPersistorConfig {
  url?: unknown;
  apiKey?: unknown;
  timeout?: unknown;
  searchMode?: unknown;
  searchLimit?: unknown;
}

interface RawWeightsConfig {
  file?: unknown;
  persistor?: unknown;
}

type SearchMode = 'hybrid' | 'text' | 'semantic';
const VALID_SEARCH_MODES = new Set<SearchMode>(['hybrid', 'text', 'semantic']);

function isSearchMode(v: string): v is SearchMode {
  return VALID_SEARCH_MODES.has(v as SearchMode);
}

const isPositiveFinite = (v: unknown): v is number =>
  typeof v === 'number' && Number.isFinite(v) && v > 0;

/**
 * Merge partial user config with defaults.
 *
 * Environment variable interpolation: string values matching the pattern
 * `${ENV_VAR}` are resolved from `process.env` at startup. For example,
 * `apiKey: "${PERSISTOR_API_KEY}"` reads the key from that env var.
 * If the env var is unset, an empty string is used.
 */
export function resolveConfig(raw: Record<string, unknown>): PersistorPluginConfig {
  const isRecord = (v: unknown): v is Record<string, unknown> =>
    v != null && typeof v === 'object' && !Array.isArray(v);

  // Safe cast: RawPersistorConfig has all-optional `unknown` fields, so any Record<string, unknown> satisfies it
  const persistorRaw: RawPersistorConfig = isRecord(raw['persistor'])
    ? (raw['persistor'] as RawPersistorConfig)
    : {};
  // Safe cast: RawWeightsConfig has all-optional `unknown` fields, so any Record<string, unknown> satisfies it
  const weightsRaw: RawWeightsConfig = isRecord(raw['weights'])
    ? (raw['weights'] as RawWeightsConfig)
    : {};

  const rawUrl = persistorRaw.url;
  const url = typeof rawUrl === 'string' ? rawUrl : defaultConfig.persistor.url;

  const rawApiKey = persistorRaw.apiKey;
  let apiKey = typeof rawApiKey === 'string' ? rawApiKey : '';
  if (apiKey.startsWith('${') && apiKey.endsWith('}')) {
    const envVar = apiKey.slice(2, -1);
    apiKey = process.env[envVar] ?? '';
  } else if (!apiKey) {
    apiKey = process.env['PERSISTOR_API_KEY'] ?? '';
  }

  const rawTimeout = persistorRaw.timeout;
  const timeout = isPositiveFinite(rawTimeout) ? rawTimeout : defaultConfig.persistor.timeout;

  const rawSearchMode = persistorRaw.searchMode;
  const searchMode =
    typeof rawSearchMode === 'string' && isSearchMode(rawSearchMode)
      ? rawSearchMode
      : defaultConfig.persistor.searchMode;

  const rawSearchLimit = persistorRaw.searchLimit;
  const searchLimit = isPositiveFinite(rawSearchLimit)
    ? rawSearchLimit
    : defaultConfig.persistor.searchLimit;

  const rawFileWeight = weightsRaw.file;
  const fileWeight = typeof rawFileWeight === 'number' ? rawFileWeight : defaultConfig.weights.file;

  const rawPersistorWeight = weightsRaw.persistor;
  const persistorWeight =
    typeof rawPersistorWeight === 'number' ? rawPersistorWeight : defaultConfig.weights.persistor;

  const rawContextOnGet = raw['persistorContextOnGet'];
  const persistorContextOnGet =
    typeof rawContextOnGet === 'boolean' ? rawContextOnGet : defaultConfig.persistorContextOnGet;

  return {
    persistor: { url, apiKey, timeout, searchMode, searchLimit },
    weights: { file: fileWeight, persistor: persistorWeight },
    persistorContextOnGet,
  };
}
