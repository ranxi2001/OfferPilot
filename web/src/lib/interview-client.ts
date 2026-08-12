import type {
  AnswerInterviewRequest,
  AnswerInterviewResponse,
  InterviewApiError,
  InterviewExecutionTrace,
  InterviewReport,
  InterviewReviewSnapshot,
  InterviewSessionEventPage,
  InterviewSnapshot,
  ReportInterviewRequest,
  StartInterviewRequest,
  StartInterviewResponse,
} from '@/types/interview';

type TraceListener = (trace: InterviewExecutionTrace) => void;

interface TraceEnvelope {
  type: 'trace';
  trace: InterviewExecutionTrace;
}

interface ResultEnvelope<T> {
  type: 'result';
  status: number;
  data: T | InterviewApiError;
}

type StreamEnvelope<T> = TraceEnvelope | ResultEnvelope<T>;

export class InterviewRequestError extends Error {
  readonly code: string;
  readonly retryable: boolean;
  readonly status: number;

  constructor(message: string, options: { code?: string; retryable?: boolean; status?: number } = {}) {
    super(message);
    this.name = 'InterviewRequestError';
    this.code = options.code ?? 'interview_request_failed';
    this.retryable = options.retryable ?? false;
    this.status = options.status ?? 500;
  }
}

async function postInterview<T>(payload: object, onTrace?: TraceListener): Promise<T> {
  let response: Response;
  try {
    response = await fetch('/api/interview', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/x-ndjson',
      },
      body: JSON.stringify(payload),
    });
  } catch (error) {
    throw new InterviewRequestError(`无法连接面试服务：${(error as Error).message}`, {
      code: 'backend_unavailable',
      retryable: true,
      status: 502,
    });
  }

  const contentType = response.headers.get('content-type') ?? '';
  if (!contentType.includes('application/x-ndjson') || !response.body) {
    const data = await readJSONResponse<T>(response);
    if (!response.ok) throw requestError(data, response.status);
    return data as T;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let result: ResultEnvelope<T> | null = null;

  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      const lines = buffer.split('\n');
      buffer = done ? '' : (lines.pop() ?? '');
      for (const line of lines) {
        if (!line.trim()) continue;
        const envelope = JSON.parse(line) as StreamEnvelope<T>;
        if (envelope.type === 'trace') onTrace?.(envelope.trace);
        if (envelope.type === 'result') result = envelope;
      }
      if (done) break;
    }
  } catch (error) {
    if (error instanceof InterviewRequestError) throw error;
    throw new InterviewRequestError(`执行轨迹解析失败：${(error as Error).message}`, {
      code: 'stream_interrupted',
      retryable: true,
      status: response.status,
    });
  } finally {
    reader.releaseLock();
  }

  if (!result) {
    throw new InterviewRequestError('面试服务的执行轨迹意外中断，请重试。', {
      code: 'stream_interrupted',
      retryable: true,
      status: response.status,
    });
  }
  if (result.status < 200 || result.status >= 300) {
    throw requestError(result.data, result.status);
  }
  return result.data as T;
}

async function getInterview<T>(query: URLSearchParams, signal?: AbortSignal): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api/interview?${query.toString()}`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      cache: 'no-store',
      signal,
    });
  } catch (error) {
    if ((error as Error).name === 'AbortError') throw error;
    throw new InterviewRequestError(`无法连接面试服务：${(error as Error).message}`, {
      code: 'backend_unavailable',
      retryable: true,
      status: 502,
    });
  }

  const data = await readJSONResponse<T>(response);
  if (!response.ok) throw requestError(data, response.status);
  return data as T;
}

async function readJSONResponse<T>(response: Response): Promise<T | InterviewApiError> {
  try {
    return await response.json() as T | InterviewApiError;
  } catch {
    return { error: { code: 'invalid_response', message: `Interview request failed (${response.status})`, retryable: true } };
  }
}

function requestError(data: unknown, status: number): InterviewRequestError {
  const payload = data as InterviewApiError | undefined;
  const error = payload?.error;
  if (typeof error === 'string') return new InterviewRequestError(error, { status });
  return new InterviewRequestError(error?.message || `Interview request failed (${status})`, {
    code: error?.code,
    retryable: error?.retryable,
    status,
  });
}

export const interviewClient = {
  start: (request: StartInterviewRequest, onTrace?: TraceListener) => postInterview<StartInterviewResponse>(request, onTrace),
  answer: (request: AnswerInterviewRequest, onTrace?: TraceListener) => postInterview<AnswerInterviewResponse>(request, onTrace),
  report: (request: ReportInterviewRequest, onTrace?: TraceListener) => postInterview<InterviewReport>(request, onTrace),
  snapshot: (interviewId: string, signal?: AbortSignal) => getInterview<InterviewSnapshot>(new URLSearchParams({ interviewId }), signal),
  review: (interviewId: string, signal?: AbortSignal) => getInterview<InterviewReviewSnapshot>(new URLSearchParams({ interviewId, resource: 'review' }), signal),
  events: (interviewId: string, after = 0, limit = 100, signal?: AbortSignal) => getInterview<InterviewSessionEventPage>(new URLSearchParams({
    interviewId,
    resource: 'events',
    after: String(after),
    limit: String(limit),
  }), signal),
};
