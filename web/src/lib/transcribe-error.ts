export interface TranscribeErrorPayload {
  error: string;
  retryable: boolean;
}

interface ParsedUpstreamError {
  message?: string;
  retryable?: boolean;
}

const MAX_PUBLIC_MESSAGE_LENGTH = 240;
const DEFAULT_ERROR = '语音识别失败，请重试';

export function normalizeTranscribeError(body: string, status: number): TranscribeErrorPayload {
  const parsed = parseUpstreamError(body);
  return {
    error: publicMessage(parsed?.message, status),
    retryable: parsed?.retryable ?? isRetryableStatus(status),
  };
}

export function transcribeUnavailableError(): TranscribeErrorPayload {
  return {
    error: '无法连接语音识别服务，请稍后重试',
    retryable: true,
  };
}

function parseUpstreamError(body: string): ParsedUpstreamError | null {
  let value: unknown;
  try {
    value = JSON.parse(body);
  } catch {
    return null;
  }

  if (!isRecord(value)) return null;

  const nested = isRecord(value.error) ? value.error : null;
  const message = stringValue(nested?.message)
    ?? stringValue(value.error)
    ?? stringValue(value.message);
  const retryable = booleanValue(nested?.retryable) ?? booleanValue(value.retryable);

  return { message, retryable };
}

function publicMessage(message: string | undefined, status: number): string {
  if (message) {
    const knownMessage = translateKnownMessage(message);
    if (knownMessage) return knownMessage;

    const normalized = message.trim();
    if (isSafePublicMessage(normalized)) return normalized;
  }

  if (status === 408 || status === 504) return '语音识别超时，请重试';
  if (status === 413) return '录音文件过大，请缩短回答后重试';
  if (status === 429) return '语音识别请求过多，请稍后重试';
  if (status === 401 || status === 403) return '语音识别服务配置异常，请联系管理员';
  if (status >= 500) return '语音识别服务暂时不可用，请重试';
  return DEFAULT_ERROR;
}

function translateKnownMessage(message: string): string | null {
  if (/audio body is required/i.test(message)) return '没有收到录音，请重新录制';
  if (/supports only WAV or MP3|unsupported (?:audio|file) format/i.test(message)) {
    return '当前录音格式无法识别，请重新录制';
  }
  if (/returned an empty transcript|empty transcript/i.test(message)) {
    return '未识别到有效语音，请重试';
  }
  if (/speech model is not configured|MIMO_API_KEY is required/i.test(message)) {
    return '语音识别服务尚未配置，请联系管理员';
  }
  return null;
}

function isSafePublicMessage(message: string): boolean {
  if (!message || message.length > MAX_PUBLIC_MESSAGE_LENGTH) return false;
  if (/\r|\n|\t/.test(message)) return false;
  return !/(?:https?|wss?):\/\/|\blocalhost\b|\b127\.0\.0\.1\b|\b(?:api[_-]?key|authorization|bearer)\b|\brequest\s*:\s*(?:get|post|put|patch|delete)\b|\bEOF\b|\b(?:MiMo|xiaomi|openai|anthropic)\b|\bspeech\s*:|<\/?html\b/i.test(message);
}

function isRetryableStatus(status: number): boolean {
  return status === 408 || status === 425 || status === 429 || status >= 500;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function booleanValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}
