import { NextRequest, NextResponse } from 'next/server';

export const MAX_JSON_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_JSON_BODY_BYTES', 256 * 1024);
export const MAX_INTERVIEW_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_INTERVIEW_BODY_BYTES', 2 * 1024 * 1024);
export const MAX_AUDIO_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_AUDIO_BODY_BYTES', 25 * 1024 * 1024);
export const MAX_UPLOAD_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_UPLOAD_BODY_BYTES', 10 * 1024 * 1024);
export const MAX_RESUME_DIAGNOSIS_BODY_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_RESUME_DIAGNOSIS_BODY_BYTES', 12 * 1024 * 1024);
export const MAX_URL_RESPONSE_BYTES = readPositiveIntEnv('OFFERPILOT_MAX_URL_RESPONSE_BYTES', 1024 * 1024);

export type BodyResult<T> =
  | { data: T; response?: never }
  | { data?: never; response: NextResponse };

export function readPositiveIntEnv(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

export function payloadTooLarge(label: string, maxBytes: number): NextResponse {
  return NextResponse.json(
    { error: `${label} exceeds ${maxBytes} bytes` },
    { status: 413 },
  );
}

export function rejectIfContentLengthExceeds(
  req: NextRequest,
  maxBytes: number,
  label: string,
): NextResponse | null {
  const raw = req.headers.get('content-length');
  if (!raw) return null;
  const length = parseInt(raw, 10);
  if (Number.isFinite(length) && length > maxBytes) {
    return payloadTooLarge(label, maxBytes);
  }
  return null;
}

export async function readJsonBody<T>(
  req: NextRequest,
  maxBytes = MAX_JSON_BODY_BYTES,
  label = 'JSON body',
): Promise<BodyResult<T>> {
  const contentLengthError = rejectIfContentLengthExceeds(req, maxBytes, label);
  if (contentLengthError) return { response: contentLengthError };

  const text = await req.text();
  if (byteLength(text) > maxBytes) {
    return { response: payloadTooLarge(label, maxBytes) };
  }

  try {
    return { data: JSON.parse(text) as T };
  } catch {
    return {
      response: NextResponse.json({ error: 'Invalid JSON' }, { status: 400 }),
    };
  }
}

export async function readTextBody(
  req: NextRequest,
  maxBytes = MAX_JSON_BODY_BYTES,
  label = 'request body',
): Promise<BodyResult<string>> {
  const contentLengthError = rejectIfContentLengthExceeds(req, maxBytes, label);
  if (contentLengthError) return { response: contentLengthError };

  const text = await req.text();
  if (byteLength(text) > maxBytes) {
    return { response: payloadTooLarge(label, maxBytes) };
  }
  return { data: text };
}

export async function readArrayBufferBody(
  req: NextRequest,
  maxBytes = MAX_AUDIO_BODY_BYTES,
  label = 'request body',
): Promise<BodyResult<ArrayBuffer>> {
  const contentLengthError = rejectIfContentLengthExceeds(req, maxBytes, label);
  if (contentLengthError) return { response: contentLengthError };

  const buffer = await req.arrayBuffer();
  if (buffer.byteLength > maxBytes) {
    return { response: payloadTooLarge(label, maxBytes) };
  }
  return { data: buffer };
}

export function byteLength(text: string): number {
  return new TextEncoder().encode(text).byteLength;
}
