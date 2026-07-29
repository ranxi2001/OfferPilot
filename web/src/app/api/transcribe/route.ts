import { NextRequest, NextResponse } from 'next/server';
import { readArrayBufferBody } from '@/lib/api-security';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;

export async function POST(req: NextRequest) {
  const parsed = await readArrayBufferBody(req, undefined, 'audio body');
  if (parsed.response) return parsed.response;
  const audio = parsed.data;

  const backendRes = await fetch(`${BACKEND_URL}/api/transcribe`, {
    method: 'POST',
    headers: {
      'Content-Type': req.headers.get('content-type') ?? 'audio/webm',
      ...(req.headers.get('x-file-name') ? { 'X-File-Name': req.headers.get('x-file-name')! } : {}),
      ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
    },
    body: audio,
    signal: req.signal,
  });

  const text = await backendRes.text();
  if (!backendRes.ok) {
    return NextResponse.json(
      { error: text || `Backend error: ${backendRes.status}` },
      { status: backendRes.status },
    );
  }

  return new Response(text, {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}
