import { afterEach, describe, expect, it, vi } from 'vitest';
import { InterviewRequestError, interviewClient } from '../../web/src/lib/interview-client.js';

const encoder = new TextEncoder();

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('interview streaming client', () => {
  it('preserves trace events split across network chunks before returning the result', async () => {
    const responseBody = [
      '{"type":"trace","trace":{"id":"load","stage":"session","label":"装载会话","detail":"已恢复","status":"completed","at":"2026-08-11T00:00:00Z"}}\n',
      '{"type":"trace","trace":{"id":"agent-1","stage":"agent","label":"评估回答","detail":"正在运行","status":"running","agent":"assessor","at":"2026-08-11T00:00:01Z"}}\n',
      '{"type":"result","status":200,"data":{"interviewId":"interview-1","state":"completed","feedback":{"questionId":"question-1","score":80,"verdict":"strong","summary":"ok","strengths":[],"gaps":[],"claimChecks":[],"coachTip":"ok"},"nextQuestion":null,"progress":{"answered":1,"target":1,"current":1,"percent":100},"reportReady":true}}\n',
    ].join('');
    const chunks = [responseBody.slice(0, 73), responseBody.slice(73, 219), responseBody.slice(219)];
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new ReadableStream({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
        controller.close();
      },
    }), { headers: { 'Content-Type': 'application/x-ndjson; charset=utf-8' } })));

    const traces: string[] = [];
    const result = await interviewClient.answer({
      action: 'answer',
      interviewId: 'interview-1',
      questionId: 'question-1',
      answer: { text: 'answer', inputMode: 'text' },
    }, (trace) => traces.push(`${trace.id}:${trace.status}`));

    expect(traces).toEqual(['load:completed', 'agent-1:running']);
    expect(result.reportReady).toBe(true);
  });

  it('surfaces a retryable domain error from the final stream envelope', async () => {
    const body = '{"type":"result","status":503,"data":{"error":{"code":"service_unavailable","message":"interview assessment is temporarily unavailable","retryable":true}}}\n';
    vi.stubGlobal('fetch', vi.fn(async () => new Response(body, {
      headers: { 'Content-Type': 'application/x-ndjson; charset=utf-8' },
    })));

    const request = interviewClient.answer({
      action: 'answer',
      interviewId: 'interview-1',
      questionId: 'question-1',
      answer: { text: 'answer', inputMode: 'text' },
    });

    await expect(request).rejects.toMatchObject<Partial<InterviewRequestError>>({
      code: 'service_unavailable',
      retryable: true,
      status: 503,
    });
  });
});
