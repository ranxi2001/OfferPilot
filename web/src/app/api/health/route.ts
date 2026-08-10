import { NextResponse } from 'next/server';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const HEALTH_TIMEOUT_MS = readPositiveIntEnv('OFFERPILOT_HEALTH_TIMEOUT_MS', 3000);

export const dynamic = 'force-dynamic';

export async function GET() {
  const started = Date.now();
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), HEALTH_TIMEOUT_MS);

  try {
    const response = await fetch(`${BACKEND_URL}/health/ready`, {
      cache: 'no-store',
      signal: controller.signal,
    });
    const body = await response.json().catch(() => null);
    const ok = response.ok;

    return NextResponse.json(
      {
        status: ok ? 'ok' : 'degraded',
        web: 'ok',
        backend: {
          ok,
          status: response.status,
          body,
        },
        latencyMs: Date.now() - started,
      },
      { status: ok ? 200 : 503 },
    );
  } catch (err) {
    return NextResponse.json(
      {
        status: 'degraded',
        web: 'ok',
        backend: {
          ok: false,
          error: (err as Error).message,
        },
        latencyMs: Date.now() - started,
      },
      { status: 503 },
    );
  } finally {
    clearTimeout(timeout);
  }
}

function readPositiveIntEnv(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}
