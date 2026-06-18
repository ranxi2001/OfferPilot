import './env.js';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { createApp } from './app.js';
import { openDatabase, initSchema } from './db/index.js';
import { transcribeAudio, synthesizeSpeech } from './realtime/mimo-audio.js';
import { resolve } from 'node:path';
import { logger } from './logger.js';

const PORT = parseInt(process.env.PORT ?? '3001', 10);
const API_KEY = process.env.OFFERPILOT_API_KEY;
const DB_PATH = resolve(process.env.DB_PATH ?? 'data/agent.db');
const HEARTBEAT_INTERVAL = 15000;

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
  if (!API_KEY) return true;
  const authHeader = req.headers.authorization;
  if (!authHeader) return false;
  const token = authHeader.replace(/^Bearer\s+/i, '');
  return token === API_KEY;
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => resolve(Buffer.concat(chunks).toString()));
    req.on('error', reject);
  });
}

function readBodyBuffer(req: IncomingMessage): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on('data', (chunk) => chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)));
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

function cors(res: ServerResponse): void {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-File-Name');
}

const db = openDatabase(DB_PATH);
initSchema(db);

const server = createServer(async (req, res) => {
  cors(res);

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  if (req.url === '/api/chat' && req.method === 'POST') {
    if (!validateAuth(req)) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Unauthorized' }));
      return;
    }

    let body: { message?: string; sessionId?: string; model?: string };
    try {
      body = JSON.parse(await readBody(req));
    } catch {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Invalid JSON' }));
      return;
    }

    if (!body.message) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'message is required' }));
      return;
    }

    res.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    });

    const send = (event: Record<string, unknown>) => {
      res.write(`data: ${JSON.stringify(event)}\n\n`);
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
        model: body.model,
        onTextDelta: (text) => send({ type: 'text_delta', content: text }),
        onThinkingDelta: (text) => send({ type: 'thinking_delta', content: text }),
        onToolCall: (name, input) => send({ type: 'tool_call', name, input }),
        onToolResult: (name, result) => send({ type: 'tool_result', name, result }),
      });

      const session = body.sessionId
        ? app.sessionManager.get(body.sessionId) ?? app.sessionManager.create()
        : app.sessionManager.create();

      send({ type: 'session', sessionId: session.id });

      await app.agent.run(session.id, body.message);

      send({ type: 'done', usage: app.agent.getUsage() });
    } catch (err) {
      send({ type: 'error', message: (err as Error).message });
    } finally {
      clearInterval(heartbeat);
      res.write('data: [DONE]\n\n');
      res.end();
    }
    return;
  }

  if (req.url === '/api/session' && req.method === 'POST') {
    if (!validateAuth(req)) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Unauthorized' }));
      return;
    }

    const app = createApp({});
    const session = app.sessionManager.create();
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ sessionId: session.id }));
    return;
  }

  if (req.url === '/api/transcribe' && req.method === 'POST') {
    if (!validateAuth(req)) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Unauthorized' }));
      return;
    }

    try {
      const audio = await readBodyBuffer(req);
      if (audio.length === 0) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'audio body is required' }));
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
      logger.error('transcribe failed', { error: errorMessage(err) });
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: errorMessage(err) }));
    }
    return;
  }

  if (req.url === '/api/tts' && req.method === 'POST') {
    if (!validateAuth(req)) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Unauthorized' }));
      return;
    }

    try {
      const body = JSON.parse(await readBody(req)) as { text?: string; voice?: string; format?: string };
      if (!body.text) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'text is required' }));
        return;
      }

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
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: errorMessage(err) }));
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
