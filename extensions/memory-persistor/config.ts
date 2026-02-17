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

interface RawPluginConfig {
  persistor?: unknown;
  weights?: unknown;
  persistorContextOnGet?: unknown;
}

const VALID_SEARCH_MODES = new Set<string>(['hybrid', 'text', 'semantic']);

/**
 * Merge partial user config with defaults.
 *
 * Environment variable interpolation: string values matching the pattern
 * `${ENV_VAR}` are resolved from `process.env` at startup. For example,
 * `apiKey: "${PERSISTOR_API_KEY}"` reads the key from that env var.
 * If the env var is unset, an empty string is used.
 */
export function resolveConfig(raw: Record<string, unknown>): PersistorPluginConfig {
  const pluginRaw = raw as RawPluginConfig;
  const persistorRaw: RawPersistorConfig =
    pluginRaw.persistor != null && typeof pluginRaw.persistor === 'object'
      ? (pluginRaw.persistor as RawPersistorConfig)
      : {};
  const weightsRaw: RawWeightsConfig =
    pluginRaw.weights != null && typeof pluginRaw.weights === 'object'
      ? (pluginRaw.weights as RawWeightsConfig)
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
  const timeout = typeof rawTimeout === 'number' ? rawTimeout : defaultConfig.persistor.timeout;

  const rawSearchMode = persistorRaw.searchMode;
  const searchMode =
    typeof rawSearchMode === 'string' && VALID_SEARCH_MODES.has(rawSearchMode)
      ? (rawSearchMode as 'hybrid' | 'text' | 'semantic')
      : defaultConfig.persistor.searchMode;

  const rawSearchLimit = persistorRaw.searchLimit;
  const searchLimit =
    typeof rawSearchLimit === 'number' ? rawSearchLimit : defaultConfig.persistor.searchLimit;

  const rawFileWeight = weightsRaw.file;
  const fileWeight = typeof rawFileWeight === 'number' ? rawFileWeight : defaultConfig.weights.file;

  const rawPersistorWeight = weightsRaw.persistor;
  const persistorWeight =
    typeof rawPersistorWeight === 'number' ? rawPersistorWeight : defaultConfig.weights.persistor;

  const rawContextOnGet = pluginRaw.persistorContextOnGet;
  const persistorContextOnGet =
    typeof rawContextOnGet === 'boolean' ? rawContextOnGet : defaultConfig.persistorContextOnGet;

  return {
    persistor: { url, apiKey, timeout, searchMode, searchLimit },
    weights: { file: fileWeight, persistor: persistorWeight },
    persistorContextOnGet,
  };
}
