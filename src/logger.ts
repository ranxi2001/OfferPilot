export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

const LEVEL_ORDER: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3 };

export interface LogEntry {
  level: LogLevel;
  msg: string;
  ts: string;
  [key: string]: unknown;
}

export class Logger {
  private minLevel: LogLevel;
  private context: Record<string, unknown>;

  constructor(opts?: { level?: LogLevel; context?: Record<string, unknown> }) {
    this.minLevel = opts?.level ?? (process.env.LOG_LEVEL as LogLevel) ?? 'info';
    this.context = opts?.context ?? {};
  }

  child(context: Record<string, unknown>): Logger {
    return new Logger({ level: this.minLevel, context: { ...this.context, ...context } });
  }

  debug(msg: string, data?: Record<string, unknown>): void {
    this.log('debug', msg, data);
  }

  info(msg: string, data?: Record<string, unknown>): void {
    this.log('info', msg, data);
  }

  warn(msg: string, data?: Record<string, unknown>): void {
    this.log('warn', msg, data);
  }

  error(msg: string, data?: Record<string, unknown>): void {
    this.log('error', msg, data);
  }

  private log(level: LogLevel, msg: string, data?: Record<string, unknown>): void {
    if (LEVEL_ORDER[level] < LEVEL_ORDER[this.minLevel]) return;

    const entry: LogEntry = {
      level,
      msg,
      ts: new Date().toISOString(),
      ...this.context,
      ...data,
    };

    const output = JSON.stringify(entry);

    if (level === 'error') {
      process.stderr.write(output + '\n');
    } else {
      process.stdout.write(output + '\n');
    }
  }
}

export const logger = new Logger();
