import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { once } from 'node:events';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer } from 'node:net';
import Database from 'better-sqlite3';
import { afterAll, describe, expect, it } from 'vitest';

interface TestServer {
  baseUrl: string;
  child: ChildProcessWithoutNullStreams;
  tempDir: string;
  dbPath: string;
  output: () => string;
}

describe('E2E: production server startup', () => {
  let server: TestServer;

  afterAll(async () => {
    if (server) {
      await stopServer(server);
    }
  }, 10000);

  it('seeds knowledge and persists sessions through the configured database', async () => {
    server = await startServer();

    const sessionRes = await fetch(`${server.baseUrl}/api/session`, {
      method: 'POST',
      headers: { Authorization: 'Bearer production-secret' },
    });

    expect(sessionRes.status).toBe(200);

    const db = new Database(server.dbPath);
    try {
      const knowledge = db.prepare('SELECT COUNT(*) AS count FROM knowledge').get() as { count: number };
      const sessions = db.prepare('SELECT COUNT(*) AS count FROM sessions').get() as { count: number };

      expect(knowledge.count).toBeGreaterThan(0);
      expect(sessions.count).toBe(1);
    } finally {
      db.close();
    }
  }, 15000);
});

async function startServer(): Promise<TestServer> {
  const port = await getFreePort();
  const tempDir = mkdtempSync(join(tmpdir(), 'offerpilot-prod-e2e-'));
  const dbPath = join(tempDir, 'agent.db');
  const knowledgeDir = join(tempDir, 'knowledge');
  const output: string[] = [];
  const tsxCli = createRequire(join(process.cwd(), 'package.json')).resolve('tsx/cli');

  mkdirSync(knowledgeDir, { recursive: true });
  writeFileSync(
    join(knowledgeDir, 'react.md'),
    [
      '# ReAct Agent',
      '',
      '## Q: 什么是 ReAct Agent？',
      '',
      '高手答案：ReAct 是 reasoning 与 acting 交替的 Agent 循环。',
      '',
    ].join('\n'),
    'utf-8',
  );

  const child = spawn(process.execPath, [tsxCli, 'src/server.ts'], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      PORT: String(port),
      NODE_ENV: 'production',
      OFFERPILOT_API_KEY: 'production-secret',
      OFFERPILOT_ALLOWED_ORIGINS: 'http://allowed.example',
      OFFERPILOT_SEED_KNOWLEDGE_ON_START: 'true',
      KNOWLEDGE_DIR: knowledgeDir,
      DB_PATH: dbPath,
      LOG_LEVEL: 'error',
      ANTHROPIC_API_KEY: '',
      OPENAI_API_KEY: '',
      DEEPSEEK_API_KEY: '',
      MIMO_API_KEY: '',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  child.stdout.on('data', (chunk) => output.push(String(chunk)));
  child.stderr.on('data', (chunk) => output.push(String(chunk)));

  const baseUrl = `http://127.0.0.1:${port}`;
  await waitForHealth(baseUrl, child, () => output.join(''));

  return { baseUrl, child, tempDir, dbPath, output: () => output.join('') };
}

async function stopServer(server: TestServer): Promise<void> {
  if (server.child.exitCode === null) {
    server.child.kill('SIGTERM');
    await Promise.race([
      once(server.child, 'exit'),
      sleep(5000).then(() => {
        if (server.child.exitCode === null) {
          server.child.kill('SIGKILL');
        }
      }),
    ]);
  }

  rmSync(server.tempDir, { recursive: true, force: true });
}

async function waitForHealth(
  baseUrl: string,
  child: ChildProcessWithoutNullStreams,
  output: () => string,
): Promise<void> {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`server exited early with ${child.exitCode}:\n${output()}`);
    }

    try {
      const response = await fetch(`${baseUrl}/health`);
      if (response.ok) return;
    } catch {
      await sleep(100);
    }
  }

  throw new Error(`server did not become healthy:\n${output()}`);
}

function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        server.close(() => reject(new Error('failed to allocate a port')));
        return;
      }
      const port = address.port;
      server.close(() => resolve(port));
    });
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
