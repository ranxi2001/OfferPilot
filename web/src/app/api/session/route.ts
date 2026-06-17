import { NextRequest, NextResponse } from 'next/server';
import { randomUUID } from 'node:crypto';

const API_KEY = process.env.OFFERPILOT_API_KEY;

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

  const sessionId = randomUUID();
  return NextResponse.json({ sessionId });
}
