import type { InterviewExecutionRun, InterviewExecutionTrace } from '@/types/interview';

export function mergeTraceIntoRuns(
  runs: InterviewExecutionRun[],
  runId: string,
  trace: InterviewExecutionTrace,
): InterviewExecutionRun[] {
  return runs.map((run) => {
    if (run.id !== runId) return run;
    const position = run.steps.findIndex((step) => step.id === trace.id);
    const steps = [...run.steps];
    const transition = {
      status: trace.status,
      at: trace.at,
      detail: trace.detail,
      durationMs: trace.durationMs,
    };
    if (position >= 0) {
      const previous = steps[position];
      const transitions = previous.transitions ?? [{
        status: previous.status,
        at: previous.at,
        detail: previous.detail,
        durationMs: previous.durationMs,
      }];
      const last = transitions[transitions.length - 1];
      steps[position] = {
        ...trace,
        transitions: last?.status === transition.status && last?.at === transition.at
          ? transitions
          : [...transitions, transition],
      };
    } else {
      steps.push({ ...trace, transitions: [transition] });
    }
    return { ...run, steps };
  });
}

export function failExecutionInRuns(
  runs: InterviewExecutionRun[],
  runId: string,
  message: string,
  finishedAt: string,
): InterviewExecutionRun[] {
  return runs.map((run) => {
    if (run.id !== runId) return run;
    let hasFailedStep = false;
    const steps = run.steps.map((step) => {
      if (step.status === 'failed') hasFailedStep = true;
      if (step.status !== 'running' && step.status !== 'queued') return step;
      hasFailedStep = true;
      const transitions = step.transitions ?? [{
        status: step.status,
        at: step.at,
        detail: step.detail,
        durationMs: step.durationMs,
      }];
      return {
        ...step,
        status: 'failed' as const,
        detail: message,
        at: finishedAt,
        transitions: [...transitions, { status: 'failed' as const, at: finishedAt, detail: message }],
      };
    });
    if (!hasFailedStep) {
      steps.push({
        id: `${run.id}:failed`,
        stage: 'request',
        label: '本次执行未提交',
        detail: message,
        status: 'failed',
        at: finishedAt,
        transitions: [{ status: 'failed', at: finishedAt, detail: message }],
      });
    }
    return { ...run, status: 'failed', finishedAt, steps };
  });
}
