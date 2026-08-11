import { describe, expect, it, vi } from 'vitest';
import {
  VoiceAnswerRecordingCache,
  VoiceTranscriptionError,
  voiceTranscriptionErrorMessage,
  type VoiceTranscriptionFetch,
} from '../../web/src/lib/voice-answer-recording.js';

describe('voice answer recording cache', () => {
  it('reuses the exact cached Blob and original duration when transcription is retried', async () => {
    const blob = new Blob(['the same wav bytes'], { type: 'audio/wav' });
    const requestBodies: BodyInit[] = [];
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requestBodies.push(init?.body as BodyInit);
      return new Response(JSON.stringify({ text: '缓存中的回答' }), {
        headers: { 'Content-Type': 'application/json' },
      });
    }) as VoiceTranscriptionFetch;
    const cache = new VoiceAnswerRecordingCache(fetchMock);
    const recording = cache.cache(blob, 4321.4, 'interview-1', 'question-1');

    const first = await cache.transcribe('interview-1', 'question-1');
    const retry = await cache.transcribe('interview-1', 'question-1');

    expect(first?.recording).toBe(recording);
    expect(retry?.recording).toBe(recording);
    expect(retry?.recording.durationMs).toBe(4321.4);
    expect(requestBodies).toEqual([blob, blob]);
    expect(requestBodies[0]).toBe(requestBodies[1]);
  });

  it('does not return an older transcription after a new recording replaces it', async () => {
    let resolveFirst: ((response: Response) => void) | undefined;
    const fetchMock = vi.fn()
      .mockImplementationOnce(() => new Promise<Response>((resolve) => {
        resolveFirst = resolve;
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ text: '新的回答' })));
    const cache = new VoiceAnswerRecordingCache(fetchMock as VoiceTranscriptionFetch);
    cache.cache(new Blob(['old']), 1000, 'interview-1', 'question-1');
    const oldRequest = cache.transcribe('interview-1', 'question-1');

    const newRecording = cache.cache(new Blob(['new']), 2000, 'interview-1', 'question-1');
    const newRequest = cache.transcribe('interview-1', 'question-1');
    resolveFirst?.(new Response(JSON.stringify({ text: '过期回答' })));

    await expect(oldRequest).resolves.toBeNull();
    await expect(newRequest).resolves.toEqual({ recording: newRecording, text: '新的回答' });
  });

  it('lets only the latest concurrent retry return while keeping the same Blob', async () => {
    let resolveFirst: ((response: Response) => void) | undefined;
    const bodies: Array<BodyInit | null | undefined> = [];
    const fetchMock = vi.fn()
      .mockImplementationOnce((_input: RequestInfo | URL, init?: RequestInit) => {
        bodies.push(init?.body);
        return new Promise<Response>((resolve) => {
          resolveFirst = resolve;
        });
      })
      .mockImplementationOnce((_input: RequestInfo | URL, init?: RequestInit) => {
        bodies.push(init?.body);
        return Promise.resolve(new Response(JSON.stringify({ text: '有效回答' })));
      });
    const cache = new VoiceAnswerRecordingCache(fetchMock as VoiceTranscriptionFetch);
    const blob = new Blob(['same recording']);
    const recording = cache.cache(blob, 900, 'interview-1', 'question-1');

    const olderRetry = cache.transcribe('interview-1', 'question-1');
    const latestRetry = cache.transcribe('interview-1', 'question-1');
    resolveFirst?.(new Response(JSON.stringify({ text: '过期回答' })));

    await expect(olderRetry).resolves.toBeNull();
    await expect(latestRetry).resolves.toEqual({ recording, text: '有效回答' });
    expect(bodies).toEqual([blob, blob]);
  });

  it('binds a recording to one interview question and clears it explicitly', async () => {
    const cache = new VoiceAnswerRecordingCache(vi.fn() as VoiceTranscriptionFetch);
    const recording = cache.cache(new Blob(['answer']), 800, 'interview-1', 'question-1');

    expect(cache.currentFor('interview-1', 'question-1')).toBe(recording);
    expect(cache.currentFor('interview-1', 'question-2')).toBeNull();
    expect(await cache.transcribe('interview-1', 'question-2')).toBeNull();

    cache.clear();
    expect(cache.currentFor('interview-1', 'question-1')).toBeNull();
  });

  it('hides nested provider errors and marks a 5xx response as retryable', async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ error: JSON.stringify({ error: 'speech: MiMo ASR request: EOF' }) }),
      { status: 502 },
    )) as VoiceTranscriptionFetch;
    const cache = new VoiceAnswerRecordingCache(fetchMock);
    cache.cache(new Blob(['answer']), 800, 'interview-1', 'question-1');

    const request = cache.transcribe('interview-1', 'question-1');

    await expect(request).rejects.toMatchObject({
      message: '语音服务暂时不可用，请稍后重试',
      retryable: true,
      status: 502,
    });
    expect(voiceTranscriptionErrorMessage('{"error":"provider URL: EOF"}')).toBe('语音服务暂时不可用');
  });

  it('does not offer an endless retry for a non-retryable 4xx response', async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ error: 'request too large', retryable: false }),
      { status: 413 },
    )) as VoiceTranscriptionFetch;
    const cache = new VoiceAnswerRecordingCache(fetchMock);
    cache.cache(new Blob(['answer']), 800, 'interview-1', 'question-1');

    const request = cache.transcribe('interview-1', 'question-1');

    await expect(request).rejects.toBeInstanceOf(VoiceTranscriptionError);
    await expect(request).rejects.toMatchObject({
      message: '录音文件过大，请重新录制较短的回答',
      retryable: false,
      status: 413,
    });
  });

  it('honors an explicit retryable marker on a 4xx response', async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ error: JSON.stringify({ retryable: true, message: 'temporary' }) }),
      { status: 400 },
    )) as VoiceTranscriptionFetch;
    const cache = new VoiceAnswerRecordingCache(fetchMock);
    cache.cache(new Blob(['answer']), 800, 'interview-1', 'question-1');

    await expect(cache.transcribe('interview-1', 'question-1')).rejects.toMatchObject({
      retryable: true,
      status: 400,
    });
  });

  it('honors an explicit non-retryable marker on a 5xx response', async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ error: '语音识别服务配置异常', retryable: false }),
      { status: 503 },
    )) as VoiceTranscriptionFetch;
    const cache = new VoiceAnswerRecordingCache(fetchMock);
    cache.cache(new Blob(['answer']), 800, 'interview-1', 'question-1');

    await expect(cache.transcribe('interview-1', 'question-1')).rejects.toMatchObject({
      message: '语音服务配置异常，请联系管理员',
      retryable: false,
      status: 503,
    });
  });
});
