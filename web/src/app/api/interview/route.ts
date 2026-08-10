import { NextRequest, NextResponse } from 'next/server';
import { MAX_INTERVIEW_BODY_BYTES, readJsonBody } from '@/lib/api-security';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;

export async function POST(req: NextRequest) {
  const wantsTraceStream = req.headers.get('accept')?.includes('application/x-ndjson') ?? false;
  const parsed = await readJsonBody<Record<string, unknown>>(
    req,
    MAX_INTERVIEW_BODY_BYTES,
    'interview request body',
  );
  if (parsed.response) return parsed.response;

  try {
    const backendPath = wantsTraceStream ? '/api/interview/stream' : '/api/interview';
    const response = await fetch(`${BACKEND_URL}${backendPath}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(wantsTraceStream ? { Accept: 'application/x-ndjson' } : {}),
        ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      },
      body: JSON.stringify(parsed.data),
      signal: req.signal,
      cache: 'no-store',
    });
    if (wantsTraceStream && response.body) {
      return new Response(response.body, {
        status: response.status,
        headers: {
          'Content-Type': response.headers.get('content-type') ?? 'application/x-ndjson; charset=utf-8',
          'Cache-Control': 'no-store, no-transform',
          'X-Accel-Buffering': 'no',
        },
      });
    }

    const text = await response.text();
    return new Response(text, {
      status: response.status,
      headers: { 'Content-Type': 'application/json; charset=utf-8' },
    });
  } catch (err) {
    return NextResponse.json(
      { error: { code: 'backend_unavailable', message: `Go backend unavailable: ${(err as Error).message}`, retryable: true } },
      { status: 502 },
    );
  }
}
