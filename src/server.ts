import './env.js';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { timingSafeEqual } from 'node:crypto';
import { existsSync } from 'node:fs';
import { createApp } from './app.js';
import { openDatabase, initSchema } from './db/index.js';
import { KnowledgeSearch, parseKnowledgeDir } from './knowledge/index.js';
import { transcribeAudio, synthesizeSpeech } from './realtime/mimo-audio.js';
import { resolve } from 'node:path';
import { logger } from './logger.js';

const PORT = parseInt(process.env.PORT ?? '3001', 10);
const API_KEY = process.env.OFFERPILOT_API_KEY;
const DB_PATH = resolve(process.env.DB_PATH ?? 'data/agent.db');
const KNOWLEDGE_DIR = resolve(process.env.KNOWLEDGE_DIR ?? 'knowledge');
const HEARTBEAT_INTERVAL = 15000;
const AUTH_REQUIRED = process.env.NODE_ENV === 'production' || process.env.OFFERPILOT_REQUIRE_AUTH === 'true';
const SEED_KNOWLEDGE_ON_START = process.env.OFFERPILOT_SEED_KNOWLEDGE_ON_START !== 'false';
const MAX_JSON_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_JSON_BODY_BYTES', 256 * 1024);
const MAX_AUDIO_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_AUDIO_BODY_BYTES', 25 * 1024 * 1024);
const MAX_MESSAGE_CHARS = readPositiveIntEnv('OFFERPILOT_MAX_MESSAGE_CHARS', 20000);
const MAX_TTS_TEXT_CHARS = readPositiveIntEnv('OFFERPILOT_MAX_TTS_TEXT_CHARS', 5000);

class BodyTooLargeError extends Error {
  constructor(readonly maxBytes: number) {
    super(`Request body exceeds ${maxBytes} bytes`);
  }
}

function readPositiveIntEnv(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    const cause = (err as Error & { cause?: unknown }).cause;
    if (cause instanceof Error) {
      return `${err.message}: ${cause.message}`;
    }
    return err.message;
  }
  return String(err);
}

function validateAuth(req: IncomingMessage): boolean {
  if (!API_KEY) return !AUTH_REQUIRED;
  const authHeader = req.headers.authorization;
  if (!authHeader) return false;
  const token = authHeader.replace(/^Bearer\s+/i, '');
  return safeEqual(token, API_KEY);
}

function safeEqual(a: string, b: string): boolean {
  const aBuf = Buffer.from(a);
  const bBuf = Buffer.from(b);
  if (aBuf.length !== bBuf.length) return false;
  return timingSafeEqual(aBuf, bBuf);
}

function authError(): { error: string } {
  return API_KEY ? { error: 'Unauthorized' } : { error: 'Server authentication is not configured' };
}

function writeJson(res: ServerResponse, status: number, payload: Record<string, unknown>): void {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(payload));
}

function writeReadError(res: ServerResponse, err: unknown, fallback: string): void {
  if (err instanceof BodyTooLargeError) {
    writeJson(res, 413, { error: err.message });
    return;
  }
  writeJson(res, 400, { error: fallback });
}

function readBody(req: IncomingMessage, maxBytes = MAX_JSON_BODY_BYTES): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let total = 0;
    let rejected = false;
    req.on('data', (chunk) => {
      if (rejected) return;
      const buf = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      total += buf.length;
      if (total > maxBytes) {
        rejected = true;
        reject(new BodyTooLargeError(maxBytes));
        return;
      }
      chunks.push(buf);
    });
    req.on('end', () => {
      if (!rejected) resolve(Buffer.concat(chunks).toString());
    });
    req.on('error', reject);
  });
}

function readBodyBuffer(req: IncomingMessage, maxBytes = MAX_AUDIO_BODY_BYTES): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let total = 0;
    let rejected = false;
    req.on('data', (chunk) => {
      if (rejected) return;
      const buf = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      total += buf.length;
      if (total > maxBytes) {
        rejected = true;
        reject(new BodyTooLargeError(maxBytes));
        return;
      }
      chunks.push(buf);
    });
    req.on('end', () => {
      if (!rejected) resolve(Buffer.concat(chunks));
    });
    req.on('error', reject);
  });
}

function cors(req: IncomingMessage, res: ServerResponse): boolean {
  const origin = req.headers.origin;
  if (origin && !isAllowedOrigin(origin)) {
    return false;
  }

  if (origin) {
    res.setHeader('Access-Control-Allow-Origin', origin);
    res.setHeader('Vary', 'Origin');
  }
  res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-File-Name');
  return true;
}

function isAllowedOrigin(origin: string): boolean {
  const configured = process.env.OFFERPILOT_ALLOWED_ORIGINS?.split(',')
    .map((item) => item.trim())
    .filter(Boolean);
  const allowed = configured?.length ? configured : [
    'http://localhost:3000',
    'http://127.0.0.1:3000',
  ];

  return allowed.includes(origin) || (process.env.NODE_ENV !== 'production' && allowed.includes('*'));
}

const db = openDatabase(DB_PATH);
initSchema(db);
seedKnowledgeIfNeeded();

const sharedApp = createApp({ db });
const { sessionManager, memoryStore } = sharedApp;

function seedKnowledgeIfNeeded(): void {
  if (!SEED_KNOWLEDGE_ON_START) {
    logger.info('knowledge seed skipped', { reason: 'disabled' });
    return;
  }

  if (!existsSync(KNOWLEDGE_DIR)) {
    logger.warn('knowledge seed skipped', { reason: 'missing directory', path: KNOWLEDGE_DIR });
    return;
  }

  const search = new KnowledgeSearch(db);
  const existing = search.count();
  if (existing > 0) {
    logger.info('knowledge seed skipped', { reason: 'database already populated', count: existing });
    return;
  }

  const entries = parseKnowledgeDir(KNOWLEDGE_DIR);
  search.bulkInsert(entries);
  logger.info('knowledge seeded', { count: entries.length, path: KNOWLEDGE_DIR, dbPath: DB_PATH });
}

const server = createServer(async (req, res) => {
  if (!cors(req, res)) {
    writeJson(res, 403, { error: 'Origin is not allowed' });
    return;
  }

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  if (req.url === '/api/chat' && req.method === 'POST') {
    if (!validateAuth(req)) {
      writeJson(res, 401, authError());
      return;
    }

    let body: { message?: string; sessionId?: string; model?: string };
    try {
      body = JSON.parse(await readBody(req));
    } catch (err) {
      writeReadError(res, err, 'Invalid JSON');
      return;
    }

    if (!body.message) {
      writeJson(res, 400, { error: 'message is required' });
      return;
    }

    if (body.message.length > MAX_MESSAGE_CHARS) {
      writeJson(res, 413, { error: `message exceeds ${MAX_MESSAGE_CHARS} characters` });
      return;
    }

    res.writeHead(200, {
      'Content-Type': 'text/event-stream; charset=utf-8',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    });

    const abortController = new AbortController();
    const onClose = () => abortController.abort();
    res.on('close', onClose);

    const send = (event: Record<string, unknown>) => {
      if (!res.destroyed && !res.writableEnded) {
        res.write(`data: ${JSON.stringify(event)}\n\n`);
      }
    };

    const heartbeat = setInterval(() => {
      if (!res.destroyed) {
        res.write(`: ping\n\n`);
      } else {
        clearInterval(heartbeat);
      }
    }, HEARTBEAT_INTERVAL);

    try {
      const app = createApp({
        db,
        model: body.model,
        sessionManager,
        memoryStore,
        onTextDelta: (text) => send({ type: 'text_delta', content: text }),
        onThinkingDelta: (text) => send({ type: 'thinking_delta', content: text }),
        onToolCall: (name, input) => send({ type: 'tool_call', name, input }),
        onToolResult: (name, result) => send({ type: 'tool_result', name, result }),
        abortSignal: abortController.signal,
      });

      const session = body.sessionId
        ? sessionManager.get(body.sessionId) ?? sessionManager.create()
        : app.sessionManager.create();

      send({ type: 'session', sessionId: session.id });

      await app.agent.run(session.id, body.message);

      send({ type: 'done', usage: app.agent.getUsage() });
    } catch (err) {
      send({ type: 'error', message: (err as Error).message });
    } finally {
      clearInterval(heartbeat);
      res.off('close', onClose);
      if (!res.destroyed && !res.writableEnded) {
        res.write('data: [DONE]\n\n');
        res.end();
      }
    }
    return;
  }

  if (req.url === '/api/session' && req.method === 'POST') {
    if (!validateAuth(req)) {
      writeJson(res, 401, authError());
      return;
    }

    const session = sessionManager.create();
    writeJson(res, 200, { sessionId: session.id });
    return;
  }

  if (req.url === '/api/transcribe' && req.method === 'POST') {
    if (!validateAuth(req)) {
      writeJson(res, 401, authError());
      return;
    }

    try {
      const audio = await readBodyBuffer(req);
      if (audio.length === 0) {
        writeJson(res, 400, { error: 'audio body is required' });
        return;
      }

      const result = await transcribeAudio({
        audio,
        fileName: req.headers['x-file-name']?.toString(),
        contentType: req.headers['content-type'],
      });

      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(result));
    } catch (err) {
      if (err instanceof BodyTooLargeError) {
        writeJson(res, 413, { error: err.message });
        return;
      }
      logger.error('transcribe failed', { error: errorMessage(err) });
      writeJson(res, 500, { error: errorMessage(err) });
    }
    return;
  }

  if (req.url === '/api/tts' && req.method === 'POST') {
    if (!validateAuth(req)) {
      writeJson(res, 401, authError());
      return;
    }

    let body: { text?: string; voice?: string; format?: string };
    try {
      body = JSON.parse(await readBody(req)) as { text?: string; voice?: string; format?: string };
    } catch (err) {
      writeReadError(res, err, 'Invalid JSON');
      return;
    }

    if (!body.text) {
      writeJson(res, 400, { error: 'text is required' });
      return;
    }
    if (body.text.length > MAX_TTS_TEXT_CHARS) {
      writeJson(res, 413, { error: `text exceeds ${MAX_TTS_TEXT_CHARS} characters` });
      return;
    }

    try {
      const result = await synthesizeSpeech({
        text: body.text,
        voice: body.voice,
        format: body.format,
      });

      res.writeHead(200, {
        'Content-Type': result.contentType,
        'Cache-Control': 'no-store',
      });
      res.end(result.audio);
    } catch (err) {
      logger.error('tts failed', { error: errorMessage(err) });
      writeJson(res, 500, { error: errorMessage(err) });
    }
    return;
  }

  if (req.url === '/health') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'ok' }));
    return;
  }

  res.writeHead(404, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ error: 'Not found' }));
});

server.listen(PORT, () => {
  logger.info('server started', { port: PORT, auth: !!API_KEY });
});

process.on('SIGTERM', () => {
  logger.info('shutdown', { signal: 'SIGTERM' });
  db.close();
  server.close();
});

process.on('SIGINT', () => {
  logger.info('shutdown', { signal: 'SIGINT' });
  db.close();
  server.close();
});
