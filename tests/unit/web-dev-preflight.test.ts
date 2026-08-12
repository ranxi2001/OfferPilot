import { spawnSync } from 'node:child_process';
import { createServer } from 'node:net';
import { resolve } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';

const script = resolve(process.cwd(), 'web/scripts/preflight-dev.mjs');
const servers: ReturnType<typeof createServer>[] = [];

afterEach(async () => {
  await Promise.all(
    servers.splice(0).map((server) => new Promise<void>((done) => server.close(() => done()))),
  );
});

function runPreflight(port: number) {
  return spawnSync(process.execPath, [script], {
    cwd: resolve(process.cwd(), 'web'),
    encoding: 'utf8',
    env: { ...process.env, PORT: String(port) },
  });
}

describe('Web development preflight', () => {
  it('passes when the requested port is free', () => {
    const result = runPreflight(39_991);

    expect(result.status).toBe(0);
    expect(result.stdout).toContain('Development preflight passed');
  });

  it('fails clearly without terminating a process that owns the port', async () => {
    const server = createServer();
    servers.push(server);
    await new Promise<void>((done) => server.listen(0, '127.0.0.1', () => done()));
    const address = server.address();
    if (!address || typeof address === 'string') throw new Error('Expected a TCP server address');

    const result = runPreflight(address.port);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(`Port ${address.port} is already in use`);
    expect(server.listening).toBe(true);
  });
});
