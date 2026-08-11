import { describe, expect, it } from 'vitest';
import {
  clearInterviewEventSequence,
  deriveRecoveredInterviewStage,
  hasUnresolvedAnswerEvent,
  latestAnswerEventOutcome,
  mergeInterviewEvents,
  readInterviewEventSequence,
  recoveryEventAfter,
  writeInterviewEventSequence,
  type InterviewRecoveryStorage,
} from '../../web/src/lib/interview-recovery.js';
import type {
  InterviewFeedback,
  InterviewQuestion,
  InterviewSessionEvent,
  InterviewSnapshot,
  InterviewTurn,
} from '../../web/src/types/interview.js';

class MemoryStorage implements InterviewRecoveryStorage {
  private readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }
}

function question(id: string, index: number): InterviewQuestion {
  return {
    id,
    index,
    text: `question ${index}`,
    kind: 'opening',
    focus: 'knowledge',
    topic: 'Go',
    difficulty: 'hard',
    depth: 1,
    maxDepth: 3,
    evidenceRefs: [],
  };
}

const feedback: InterviewFeedback = {
  questionId: 'question-1',
  score: 80,
  verdict: 'strong',
  summary: 'good',
  strengths: [],
  gaps: [],
  claimChecks: [],
  coachTip: 'continue',
};

const firstTurn: InterviewTurn = { question: question('question-1', 1), answer: 'answer', feedback };

function snapshot(overrides: Partial<InterviewSnapshot> = {}): InterviewSnapshot {
  return {
    interviewId: 'interview-1',
    state: 'questioning',
    profile: { topics: ['Go'], projects: [] },
    currentQuestion: question('question-2', 2),
    turns: [firstTurn],
    progress: { answered: 1, target: 2, current: 2, percent: 50 },
    reportReady: false,
    ...overrides,
  };
}

function event(sequence: number, type: InterviewSessionEvent['type'], commandId = 'command-1'): InterviewSessionEvent {
  return {
    eventId: `event-${sequence}`,
    sequence,
    commandId,
    type,
    createdAt: '2026-08-11T00:00:00Z',
  };
}

describe('interview snapshot recovery', () => {
  it('returns directly to the active question when its descriptor matches', () => {
    const recovered = deriveRecoveredInterviewStage(snapshot(), 'question-2');

    expect(recovered).toMatchObject({ phase: 'questioning', question: { id: 'question-2' } });
  });

  it('restores feedback when an answer committed before the browser received the response', () => {
    const recovered = deriveRecoveredInterviewStage(snapshot(), 'question-1');

    expect(recovered).toMatchObject({
      phase: 'feedback',
      question: { id: 'question-1' },
      pendingQuestion: { id: 'question-2' },
      feedback: { questionId: 'question-1' },
    });
  });

  it('keeps the final feedback available before report generation', () => {
    const recovered = deriveRecoveredInterviewStage(snapshot({
      state: 'completed',
      currentQuestion: null,
      reportReady: true,
      progress: { answered: 1, target: 1, current: 1, percent: 100 },
    }), 'question-1');

    expect(recovered).toMatchObject({ phase: 'feedback', question: { id: 'question-1' }, pendingQuestion: null });
  });
});

describe('durable recovery event cursor', () => {
  it('keeps polling while a command has started without a terminal event', () => {
    const started = [event(1, 'answer.started')];
    expect(hasUnresolvedAnswerEvent(started)).toBe(true);
    expect(hasUnresolvedAnswerEvent(mergeInterviewEvents(started, [event(2, 'answer.committed')]))).toBe(false);
    expect(hasUnresolvedAnswerEvent(mergeInterviewEvents(started, [event(2, 'answer.failed')]))).toBe(false);
    expect(latestAnswerEventOutcome(started)).toBe('running');
    expect(latestAnswerEventOutcome(mergeInterviewEvents(started, [event(2, 'answer.committed')]))).toBe('completed');
    expect(latestAnswerEventOutcome(mergeInterviewEvents(started, [event(2, 'answer.failed')]))).toBe('failed');
  });

  it('stores only the last sequence and queries with a bounded overlap', () => {
    const storage = new MemoryStorage();
    writeInterviewEventSequence(storage, 'interview-1', 42);

    expect(readInterviewEventSequence(storage, 'interview-1')).toBe(42);
    expect(recoveryEventAfter(42)).toBe(22);
    expect(recoveryEventAfter(5)).toBe(0);
    clearInterviewEventSequence(storage, 'interview-1');
    expect(readInterviewEventSequence(storage, 'interview-1')).toBe(0);
  });
});
