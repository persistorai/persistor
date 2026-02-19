/**
 * Minimal debug logger for memory-persistor.
 *
 * Writes to stderr only — never to stdout, which is reserved for CLI tool output.
 * Debug messages are suppressed unless the DEBUG env var includes "memory-persistor"
 * (e.g. DEBUG=memory-persistor or DEBUG=*).
 */
const NAMESPACE = 'memory-persistor';

const debugEnabled =
  typeof process !== 'undefined' &&
  typeof process.env['DEBUG'] === 'string' &&
  (process.env['DEBUG'] === '*' || process.env['DEBUG'].split(',').some((s) => s.trim() === NAMESPACE));

function serialize(v: unknown): string {
  if (typeof v === 'string') return v;
  if (v instanceof Error) return v.message;
  try {
    return JSON.stringify(v);
  } catch {
    return Object.prototype.toString.call(v);
  }
}

function write(level: string, ...args: unknown[]): void {
  process.stderr.write(`[${NAMESPACE}] [${level}] ${args.map(serialize).join(' ')}\n`);
}

export const logger = {
  debug(...args: unknown[]): void {
    if (debugEnabled) write('DEBUG', ...args);
  },
  warn(...args: unknown[]): void {
    write('WARN', ...args);
  },
};
