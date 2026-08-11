import type {
  InterviewFeedback,
  InterviewQuestion,
  InterviewSessionEvent,
  InterviewSnapshot,
} from '@/types/interview';

export const INTERVIEW_EVENT_CURSOR_KEY_PREFIX = 'offerpilot.interview.event-cursor.v1';

export interface InterviewRecoveryStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

export interface RecoveredInterviewStage {
  phase: 'questioning' | 'feedback';
  question: InterviewQuestion;
  pendingQuestion: InterviewQuestion | null;
  feedback: InterviewFeedback | null;
}

export function deriveRecoveredInterviewStage(
  snapshot: InterviewSnapshot,
  storedQuestionId: string,
): RecoveredInterviewStage | null {
  const turns = Array.isArray(snapshot.turns) ? snapshot.turns : [];
  const lastTurn = turns.at(-1);

  if (snapshot.state === 'completed') {
    if (!snapshot.reportReady || !lastTurn) return null;
    return {
      phase: 'feedback',
      question: lastTurn.question,
      pendingQuestion: null,
      feedback: lastTurn.feedback,
    };
  }

  if (!snapshot.currentQuestion) return null;
  if (snapshot.currentQuestion.id === storedQuestionId) {
    return {
      phase: 'questioning',
      question: snapshot.currentQuestion,
      pendingQuestion: null,
      feedback: null,
    };
  }

  if (lastTurn?.question.id === storedQuestionId) {
    return {
      phase: 'feedback',
      question: lastTurn.question,
      pendingQuestion: snapshot.currentQuestion,
      feedback: lastTurn.feedback,
    };
  }

  return {
    phase: 'questioning',
    question: snapshot.currentQuestion,
    pendingQuestion: null,
    feedback: null,
  };
}

export function mergeInterviewEvents(
  current: InterviewSessionEvent[],
  incoming: InterviewSessionEvent[],
): InterviewSessionEvent[] {
  const byId = new Map(current.map((event) => [event.eventId, event]));
  for (const event of incoming) byId.set(event.eventId, event);
  return [...byId.values()].sort((left, right) => left.sequence - right.sequence);
}

export function hasUnresolvedAnswerEvent(events: InterviewSessionEvent[]): boolean {
  const pending = new Set<string>();
  for (const event of [...events].sort((left, right) => left.sequence - right.sequence)) {
    if (!event.commandId) continue;
    if (event.type === 'answer.started') pending.add(event.commandId);
    if (event.type === 'answer.committed' || event.type === 'answer.failed') pending.delete(event.commandId);
  }
  return pending.size > 0;
}

export function latestAnswerEventOutcome(
  events: InterviewSessionEvent[],
): 'running' | 'completed' | 'failed' | null {
  const latest = [...events]
    .sort((left, right) => left.sequence - right.sequence)
    .findLast((event) => event.type === 'answer.started'
      || event.type === 'answer.committed'
      || event.type === 'answer.failed');
  if (!latest) return null;
  if (latest.type === 'answer.started') return 'running';
  return latest.type === 'answer.committed' ? 'completed' : 'failed';
}

export function readInterviewEventSequence(
  storage: InterviewRecoveryStorage | null | undefined,
  interviewId: string,
): number {
  const key = eventCursorKey(interviewId);
  if (!storage || !key) return 0;
  try {
    const value = Number(storage.getItem(key));
    return Number.isSafeInteger(value) && value >= 0 ? value : 0;
  } catch {
    return 0;
  }
}

export function writeInterviewEventSequence(
  storage: InterviewRecoveryStorage | null | undefined,
  interviewId: string,
  sequence: number,
): void {
  const key = eventCursorKey(interviewId);
  if (!storage || !key || !Number.isSafeInteger(sequence) || sequence < 0) return;
  try {
    storage.setItem(key, String(sequence));
  } catch {
    // Recovery still works from an overlapping event query when storage is blocked.
  }
}

export function clearInterviewEventSequence(
  storage: InterviewRecoveryStorage | null | undefined,
  interviewId: string,
): void {
  const key = eventCursorKey(interviewId);
  if (!storage || !key) return;
  try {
    storage.removeItem(key);
  } catch {
    // Reset remains usable when browser storage is blocked.
  }
}

export function recoveryEventAfter(sequence: number): number {
  return Math.max(0, sequence - 20);
}

function eventCursorKey(interviewId: string): string | null {
  const normalized = interviewId.trim();
  if (!normalized || normalized.length > 200) return null;
  return `${INTERVIEW_EVENT_CURSOR_KEY_PREFIX}:${encodeURIComponent(normalized)}`;
}
