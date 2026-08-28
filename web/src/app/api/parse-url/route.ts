import { NextRequest, NextResponse } from 'next/server';
import { readJsonBody } from '@/lib/api-security';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;

export async function POST(req: NextRequest) {
  const parsed = await readJsonBody<{ url?: string }>(req);
  if (parsed.response) return parsed.response;

  const url = parsed.data.url?.trim() ?? '';
  if (!url) {
    return NextResponse.json(
      { error: { code: 'validation_error', message: 'url is required', retryable: false, field: 'url' } },
      { status: 400 },
    );
  }

  try {
    const response = await fetch(`${BACKEND_URL}/api/v1/crawl`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
        ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      },
      body: JSON.stringify({ url }),
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
  } catch {
    return NextResponse.json(
      { error: { code: 'backend_unavailable', message: '网页爬虫 Agent 暂时不可用', retryable: true } },
      { status: 502 },
    );
  }
}
