import { describe, expect, it } from 'vitest';
import { failExecutionInRuns, mergeTraceIntoRuns } from '../../web/src/lib/execution-trace.js';
import type { InterviewExecutionRun, InterviewExecutionTrace } from '../../web/src/types/interview.js';

function run(id = 'run-1'): InterviewExecutionRun {
  return {
    id,
    action: 'answer',
    title: '回答评估',
    status: 'running',
    startedAt: '2026-08-11T00:00:00Z',
    steps: [],
  };
}

function trace(status: InterviewExecutionTrace['status'], at: string): InterviewExecutionTrace {
  return {
    id: 'assessor-1',
    stage: 'agent',
    label: '评估回答',
    detail: status === 'running' ? '正在执行' : undefined,
    status,
    agent: 'assessor',
    at,
    durationMs: status === 'completed' ? 1250 : undefined,
  };
}

describe('interview execution trace history', () => {
  it('keeps every transition for the same stable step id', () => {
    let runs = [run()];
    runs = mergeTraceIntoRuns(runs, 'run-1', trace('queued', '2026-08-11T00:00:01Z'));
    runs = mergeTraceIntoRuns(runs, 'run-1', trace('running', '2026-08-11T00:00:02Z'));
    runs = mergeTraceIntoRuns(runs, 'run-1', trace('completed', '2026-08-11T00:00:03Z'));

    expect(runs[0].steps).toHaveLength(1);
    expect(runs[0].steps[0].status).toBe('completed');
    expect(runs[0].steps[0].transitions?.map((item) => item.status)).toEqual([
      'queued',
      'running',
      'completed',
    ]);
  });

  it('records the terminal error on the failed historical step', () => {
    let runs = mergeTraceIntoRuns([run()], 'run-1', trace('running', '2026-08-11T00:00:02Z'));
    runs = failExecutionInRuns(runs, 'run-1', 'Assessor 超时，请重试', '2026-08-11T00:00:05Z');

    expect(runs[0].status).toBe('failed');
    expect(runs[0].steps[0]).toMatchObject({
      status: 'failed',
      detail: 'Assessor 超时，请重试',
    });
    expect(runs[0].steps[0].transitions?.map((item) => item.status)).toEqual(['running', 'failed']);
  });

  it('updates only the retried run and preserves the previous failed run', () => {
    const failed = failExecutionInRuns([run('run-old')], 'run-old', '第一次失败', '2026-08-11T00:00:05Z')[0];
    const runs = mergeTraceIntoRuns([failed, run('run-retry')], 'run-retry', trace('running', '2026-08-11T00:00:06Z'));

    expect(runs[0]).toEqual(failed);
    expect(runs[1].steps).toHaveLength(1);
    expect(runs[1].steps[0].status).toBe('running');
  });
});
