import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

interface TestServer {
  baseUrl: string;
  child: ChildProcessWithoutNullStreams;
  tempDir: string;
  output: () => string;
}

describe('E2E: HTTP server security', () => {
  let server: TestServer;

  beforeAll(async () => {
    server = await startServer();
  }, 15000);

  afterAll(async () => {
    if (server) {
      await stopServer(server);
    }
  }, 10000);

  it('requires bearer auth for protected API routes in production', async () => {
    const unauthorized = await fetch(`${server.baseUrl}/api/session`, {
      method: 'POST',
    });

    expect(unauthorized.status).toBe(401);

    const authorized = await fetch(`${server.baseUrl}/api/session`, {
      method: 'POST',
      headers: { Authorization: 'Bearer e2e-secret' },
    });
    const body = await authorized.json() as { sessionId?: string };

    expect(authorized.status).toBe(200);
    expect(body.sessionId).toBeTruthy();
  });

  it('rejects disallowed CORS origins before serving API requests', async () => {
    const response = await fetch(`${server.baseUrl}/api/session`, {
      method: 'POST',
      headers: {
        Authorization: 'Bearer e2e-secret',
        Origin: 'https://evil.example',
      },
    });

    expect(response.status).toBe(403);
  });

  it('rejects oversized JSON request bodies', async () => {
    const response = await fetch(`${server.baseUrl}/api/chat`, {
      method: 'POST',
      headers: {
        Authorization: 'Bearer e2e-secret',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ message: 'x'.repeat(256) }),
    });

    expect(response.status).toBe(413);
  });
});

async function startServer(): Promise<TestServer> {
  const port = await getFreePort();
  const tempDir = mkdtempSync(join(tmpdir(), 'offerpilot-e2e-'));
  const output: string[] = [];
  const npx = process.platform === 'win32' ? 'npx.cmd' : 'npx';

  const child = spawn(npx, ['tsx', 'src/server.ts'], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      PORT: String(port),
      NODE_ENV: 'production',
      OFFERPILOT_API_KEY: 'e2e-secret',
      OFFERPILOT_ALLOWED_ORIGINS: 'http://allowed.example',
      OFFERPILOT_MAX_JSON_BODY_BYTES: '96',
      DB_PATH: join(tempDir, 'agent.db'),
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

  return { baseUrl, child, tempDir, output: () => output.join('') };
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
