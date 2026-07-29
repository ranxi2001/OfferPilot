import { NextRequest, NextResponse } from 'next/server';
import { readTextBody } from '@/lib/api-security';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;

export async function POST(req: NextRequest) {
  const parsed = await readTextBody(req, undefined, 'tts request body');
  if (parsed.response) return parsed.response;
  const body = parsed.data;

  const backendRes = await fetch(`${BACKEND_URL}/api/tts`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
    },
    body,
    signal: req.signal,
  });

  if (!backendRes.ok) {
    const text = await backendRes.text();
    return NextResponse.json(
      { error: text || `Backend error: ${backendRes.status}` },
      { status: backendRes.status },
    );
  }

  return new Response(backendRes.body, {
    headers: {
      'Content-Type': backendRes.headers.get('content-type') ?? 'audio/mpeg',
      'Cache-Control': 'no-store',
    },
  });
}
