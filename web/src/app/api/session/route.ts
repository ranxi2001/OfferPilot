import { NextRequest, NextResponse } from 'next/server';
import { randomUUID } from 'node:crypto';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;
const USE_MOCK = process.env.OFFERPILOT_USE_MOCK === 'true';

function validateAuth(req: NextRequest): boolean {
  if (!API_KEY) return true;
  const authHeader = req.headers.get('authorization');
  if (!authHeader) return false;
  const token = authHeader.replace(/^Bearer\s+/i, '');
  return token === API_KEY;
}

export async function POST(req: NextRequest) {
  if (!validateAuth(req)) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 });
  }

  if (USE_MOCK) {
    return NextResponse.json({ sessionId: randomUUID() });
  }

  try {
    const backendRes = await fetch(`${BACKEND_URL}/api/session`, {
      method: 'POST',
      headers: {
        ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      },
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
      headers: { 'Content-Type': 'application/json; charset=utf-8' },
    });
  } catch (err) {
    return NextResponse.json(
      { error: `Backend unavailable at ${BACKEND_URL}: ${(err as Error).message}` },
      { status: 502 },
    );
  }
}
