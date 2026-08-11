export interface CachedVoiceAnswerRecording {
  readonly blob: Blob;
  readonly durationMs: number;
  readonly interviewId: string;
  readonly questionId: string;
}

export interface VoiceTranscriptionResult {
  recording: CachedVoiceAnswerRecording;
  text: string;
}

export type VoiceTranscriptionFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

export class VoiceTranscriptionError extends Error {
  readonly retryable: boolean;
  readonly status: number | null;

  constructor(message: string, options: { retryable: boolean; status?: number | null }) {
    super(message);
    this.name = 'VoiceTranscriptionError';
    this.retryable = options.retryable;
    this.status = options.status ?? null;
  }
}

export class VoiceAnswerRecordingCache {
  private recording: CachedVoiceAnswerRecording | null = null;
  private activeRequest: { generation: number; controller: AbortController } | null = null;
  private generation = 0;

  constructor(private readonly fetchImpl: VoiceTranscriptionFetch = (...args) => fetch(...args)) {}

  cache(
    blob: Blob,
    durationMs: number,
    interviewId: string,
    questionId: string,
  ): CachedVoiceAnswerRecording {
    this.cancelActiveRequest();
    this.recording = Object.freeze({
      blob,
      durationMs: Number.isFinite(durationMs) ? Math.max(0, durationMs) : 0,
      interviewId,
      questionId,
    });
    return this.recording;
  }

  currentFor(interviewId: string, questionId: string): CachedVoiceAnswerRecording | null {
    const current = this.recording;
    return current?.interviewId === interviewId && current.questionId === questionId ? current : null;
  }

  isCurrent(recording: CachedVoiceAnswerRecording): boolean {
    return this.recording === recording;
  }

  clear(): void {
    this.cancelActiveRequest();
    this.recording = null;
  }

  async transcribe(interviewId: string, questionId: string): Promise<VoiceTranscriptionResult | null> {
    const recording = this.currentFor(interviewId, questionId);
    if (!recording) return null;

    this.cancelActiveRequest();
    const request = {
      generation: ++this.generation,
      controller: new AbortController(),
    };
    this.activeRequest = request;

    try {
      const text = await requestVoiceTranscription(recording.blob, request.controller.signal, this.fetchImpl);
      return this.isActiveRequest(request, recording) ? { recording, text } : null;
    } catch (error) {
      if (!this.isActiveRequest(request, recording)) return null;
      throw error;
    } finally {
      if (this.activeRequest === request) this.activeRequest = null;
    }
  }

  private cancelActiveRequest(): void {
    this.generation += 1;
    this.activeRequest?.controller.abort();
    this.activeRequest = null;
  }

  private isActiveRequest(
    request: { generation: number; controller: AbortController },
    recording: CachedVoiceAnswerRecording,
  ): boolean {
    return this.activeRequest === request
      && this.generation === request.generation
      && !request.controller.signal.aborted
      && this.recording === recording;
  }
}

export function voiceTranscriptionErrorMessage(error: unknown): string {
  return error instanceof VoiceTranscriptionError ? error.message : '语音服务暂时不可用';
}

async function requestVoiceTranscription(
  blob: Blob,
  signal: AbortSignal,
  fetchImpl: VoiceTranscriptionFetch,
): Promise<string> {
  let response: Response;
  try {
    response = await fetchImpl('/api/transcribe', {
      method: 'POST',
      headers: { 'Content-Type': 'audio/wav', 'X-File-Name': 'interview.wav' },
      body: blob,
      signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error;
    throw new VoiceTranscriptionError('语音服务连接失败，请稍后重试', { retryable: true });
  }

  let rawBody: string;
  try {
    rawBody = await response.text();
  } catch {
    throw new VoiceTranscriptionError('语音服务连接中断，请稍后重试', {
      retryable: true,
      status: response.status,
    });
  }
  const payload = parseJson(rawBody);
  const text = readStringField(payload, 'text');

  if (!response.ok || !text?.trim()) {
    throw transcriptionResponseError(response.status, payload);
  }
  return text.trim();
}

function transcriptionResponseError(status: number, payload: unknown): VoiceTranscriptionError {
  const explicitlyRetryable = readBooleanField(payload, 'retryable');
  const retryable = explicitlyRetryable ?? (status === 408 || status === 429 || status >= 500);

  if (status === 408) {
    return new VoiceTranscriptionError('语音服务响应超时，请稍后重试', { retryable, status });
  }
  if (status === 429) {
    return new VoiceTranscriptionError('语音服务繁忙，请稍后重试', { retryable, status });
  }
  if (status >= 500 && explicitlyRetryable === false) {
    return new VoiceTranscriptionError('语音服务配置异常，请联系管理员', { retryable, status });
  }
  if (status >= 500) {
    return new VoiceTranscriptionError('语音服务暂时不可用，请稍后重试', { retryable, status });
  }
  if (status === 413) {
    return new VoiceTranscriptionError('录音文件过大，请重新录制较短的回答', { retryable: false, status });
  }
  if (status === 400 || status === 415 || status === 422) {
    return new VoiceTranscriptionError('录音格式无法识别，请重新录制', { retryable, status });
  }
  if (status === 401 || status === 403) {
    return new VoiceTranscriptionError('语音服务配置异常，请联系管理员', { retryable, status });
  }
  return new VoiceTranscriptionError(
    retryable ? '语音服务暂时不可用，请稍后重试' : '录音无法转写，请重新录制',
    { retryable, status },
  );
}

function parseJson(value: string): unknown {
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return value;
  }
}

function readStringField(value: unknown, key: string): string | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const field = (value as Record<string, unknown>)[key];
  return typeof field === 'string' ? field : null;
}

function readBooleanField(value: unknown, key: string, depth = 0): boolean | null {
  if (depth > 4 || value == null) return null;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return null;
    const parsed = parseJson(trimmed);
    return parsed === trimmed ? null : readBooleanField(parsed, key, depth + 1);
  }
  if (typeof value !== 'object' || Array.isArray(value)) return null;

  const record = value as Record<string, unknown>;
  if (typeof record[key] === 'boolean') return record[key];
  return readBooleanField(record.error, key, depth + 1)
    ?? readBooleanField(record.message, key, depth + 1)
    ?? readBooleanField(record.detail, key, depth + 1);
}
