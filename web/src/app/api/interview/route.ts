import { NextRequest, NextResponse } from 'next/server';
import { MAX_INTERVIEW_BODY_BYTES, readJsonBody } from '@/lib/api-security';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;

export async function GET(req: NextRequest) {
  const interviewId = req.nextUrl.searchParams.get('interviewId')?.trim() ?? '';
  if (!/^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$/.test(interviewId)) {
    return NextResponse.json(
      { error: { code: 'validation_error', message: 'interviewId is invalid', retryable: false, field: 'interviewId' } },
      { status: 400 },
    );
  }

  const resource = req.nextUrl.searchParams.get('resource');
  let backendPath = `/api/v1/interviews/${encodeURIComponent(interviewId)}`;
  if (resource === 'events') {
    const after = boundedIntegerQuery(req, 'after', 0, 0, Number.MAX_SAFE_INTEGER);
    const limit = boundedIntegerQuery(req, 'limit', 100, 1, 1000);
    if (after === null || limit === null) {
      return NextResponse.json(
        { error: { code: 'validation_error', message: 'invalid event cursor', retryable: false } },
        { status: 400 },
      );
    }
    backendPath += `/events?after=${after}&limit=${limit}`;
  } else if (resource === 'review') {
    backendPath += '/review';
  } else if (resource) {
    return NextResponse.json(
      { error: { code: 'validation_error', message: 'resource is invalid', retryable: false, field: 'resource' } },
      { status: 400 },
    );
  }

  try {
    const response = await fetch(`${BACKEND_URL}${backendPath}`, {
      method: 'GET',
      headers: {
        Accept: 'application/json',
        ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      },
      signal: req.signal,
      cache: 'no-store',
    });
    const text = await response.text();
    return new Response(text, {
      status: response.status,
      headers: {
        'Content-Type': response.headers.get('content-type') ?? 'application/json; charset=utf-8',
        'Cache-Control': 'no-store',
      },
    });
  } catch (err) {
    return NextResponse.json(
      { error: { code: 'backend_unavailable', message: `Go backend unavailable: ${(err as Error).message}`, retryable: true } },
      { status: 502 },
    );
  }
}

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

function boundedIntegerQuery(
  req: NextRequest,
  key: string,
  fallback: number,
  minimum: number,
  maximum: number,
): number | null {
  const raw = req.nextUrl.searchParams.get(key);
  if (raw === null || raw === '') return fallback;
  if (!/^\d+$/.test(raw)) return null;
  const value = Number(raw);
  return Number.isSafeInteger(value) && value >= minimum && value <= maximum ? value : null;
}
