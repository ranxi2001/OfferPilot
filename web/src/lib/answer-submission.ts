import type { AnswerInterviewRequest } from '@/types/interview';

export class AnswerSubmissionGuard {
  private generation = 0;

  begin(): number {
    this.generation += 1;
    return this.generation;
  }

  invalidate(): void {
    this.generation += 1;
  }

  isCurrent(generation: number): boolean {
    return this.generation === generation;
  }
}

export function reusableAnswerSubmission(
  pending: AnswerInterviewRequest | null,
  interviewId: string,
  questionId: string,
  answerText: string,
): AnswerInterviewRequest | null {
  if (
    pending?.interviewId !== interviewId
    || pending.questionId !== questionId
    || pending.answer.text !== answerText.trim()
  ) {
    return null;
  }
  return pending;
}
