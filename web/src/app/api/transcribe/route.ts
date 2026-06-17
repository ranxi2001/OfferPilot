import { NextRequest, NextResponse } from 'next/server';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;

export async function POST(req: NextRequest) {
  const audio = await req.arrayBuffer();

  const backendRes = await fetch(`${BACKEND_URL}/api/transcribe`, {
    method: 'POST',
    headers: {
      'Content-Type': req.headers.get('content-type') ?? 'audio/webm',
      ...(req.headers.get('x-file-name') ? { 'X-File-Name': req.headers.get('x-file-name')! } : {}),
      ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
    },
    body: audio,
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
