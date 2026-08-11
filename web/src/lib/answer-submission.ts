import type { AnswerInterviewRequest } from '@/types/interview';

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
