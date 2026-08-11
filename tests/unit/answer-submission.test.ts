import { describe, expect, it } from 'vitest';
import {
  AnswerSubmissionGuard,
  reusableAnswerSubmission,
} from '../../web/src/lib/answer-submission.js';
import type { AnswerInterviewRequest } from '../../web/src/types/interview.js';

describe('answer submission retry', () => {
  it('ignores a delayed answer result after the interview is reset', async () => {
    const guard = new AnswerSubmissionGuard();
    const generation = guard.begin();
    let release!: () => void;
    const delayedResult = new Promise<void>((resolve) => {
      release = resolve;
    });
    let phase = 'questioning';

    const applyResult = delayedResult.then(() => {
      if (guard.isCurrent(generation)) phase = 'feedback';
    });
    guard.invalidate();
    phase = 'setup';
    release();
    await applyResult;

    expect(phase).toBe('setup');
  });

  it('lets only the latest answer submission update the view', () => {
    const guard = new AnswerSubmissionGuard();
    const first = guard.begin();
    const latest = guard.begin();

    expect(guard.isCurrent(first)).toBe(false);
    expect(guard.isCurrent(latest)).toBe(true);
  });

  it('reuses the exact voice payload and duration for an unchanged retry', () => {
    const pending: AnswerInterviewRequest = {
      action: 'answer',
      interviewId: 'interview-1',
      questionId: 'question-1',
      clientAnswerId: 'answer-1',
      answer: { text: 'voice answer', inputMode: 'voice', durationMs: 4321 },
    };

    const retry = reusableAnswerSubmission(pending, 'interview-1', 'question-1', '  voice answer  ');

    expect(retry).toBe(pending);
    expect(retry?.answer).toEqual({ text: 'voice answer', inputMode: 'voice', durationMs: 4321 });
  });

  it('does not reuse an idempotency command after the answer text changes', () => {
    const pending: AnswerInterviewRequest = {
      action: 'answer',
      interviewId: 'interview-1',
      questionId: 'question-1',
      clientAnswerId: 'answer-1',
      answer: { text: 'first answer', inputMode: 'text', durationMs: 1000 },
    };

    expect(reusableAnswerSubmission(pending, 'interview-1', 'question-1', 'edited answer')).toBeNull();
  });
});
