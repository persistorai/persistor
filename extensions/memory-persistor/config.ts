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

/** Merge partial user config with defaults */
export function resolveConfig(raw: Record<string, unknown>): PersistorPluginConfig {
  const persistorRaw = (raw.persistor ?? {}) as Record<string, unknown>;
  const weightsRaw = (raw.weights ?? {}) as Record<string, unknown>;

  let apiKey = (persistorRaw.apiKey as string) ?? '';
  if (apiKey.startsWith('${') && apiKey.endsWith('}')) {
    const envVar = apiKey.slice(2, -1);
    apiKey = process.env[envVar] ?? '';
  } else if (!apiKey) {
    apiKey = process.env.PERSISTOR_API_KEY ?? '';
  }

  return {
    persistor: {
      url: (persistorRaw.url as string) ?? defaultConfig.persistor.url,
      apiKey,
      timeout: (persistorRaw.timeout as number) ?? defaultConfig.persistor.timeout,
      searchMode: (persistorRaw.searchMode as PersistorPluginConfig['persistor']['searchMode']) ?? defaultConfig.persistor.searchMode,
      searchLimit: (persistorRaw.searchLimit as number) ?? defaultConfig.persistor.searchLimit,
    },
    weights: {
      file: (weightsRaw.file as number) ?? defaultConfig.weights.file,
      persistor: (weightsRaw.persistor as number) ?? defaultConfig.weights.persistor,
    },
    persistorContextOnGet: (raw.persistorContextOnGet as boolean) ?? defaultConfig.persistorContextOnGet,
  };
}
