import { describe, expect, it } from 'vitest';
import {
  MAX_EXECUTION_HISTORY_BYTES,
  MAX_EXECUTION_HISTORY_RUNS,
  clearExecutionHistory,
  readExecutionHistory,
  serializeExecutionHistory,
  settleLatestRunningExecution,
  writeExecutionHistory,
  type ExecutionHistoryStorage,
} from '../../web/src/lib/execution-history.js';
import type { InterviewExecutionRun } from '../../web/src/types/interview.js';

class MemoryStorage implements ExecutionHistoryStorage {
  readonly values = new Map<string, string>();

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

function run(index: number): InterviewExecutionRun {
  return {
    id: `run-${index}`,
    action: 'answer',
    title: `第 ${index} 轮回答评估`,
    status: 'completed',
    startedAt: '2026-08-11T00:00:00.000Z',
    finishedAt: '2026-08-11T00:00:01.000Z',
    steps: [{
      id: `step-${index}`,
      stage: 'agent',
      label: '评估回答',
      detail: '公开的执行摘要',
      status: 'completed',
      agent: 'assessor',
      at: '2026-08-11T00:00:01.000Z',
      durationMs: 1000,
      transitions: [{ status: 'completed', at: '2026-08-11T00:00:01.000Z', durationMs: 1000 }],
    }],
  };
}

describe('safe execution history', () => {
  it('persists only the public execution field allowlist', () => {
    const unsafe = [{
      ...run(1),
      prompt: 'PRIVATE_PROMPT',
      answerText: 'PRIVATE_ANSWER',
      materials: { resume: 'PRIVATE_RESUME' },
      steps: [{
        ...run(1).steps[0],
        providerPayload: 'PRIVATE_PROVIDER_PAYLOAD',
        transitions: [{
          ...run(1).steps[0].transitions?.[0],
          chainOfThought: 'PRIVATE_REASONING',
        }],
      }],
    }];

    const serialized = serializeExecutionHistory('interview-1', unsafe);

    expect(serialized).not.toBeNull();
    expect(serialized).not.toMatch(/PRIVATE_(PROMPT|ANSWER|RESUME|PROVIDER_PAYLOAD|REASONING)/);
    expect(Object.keys(JSON.parse(serialized!).runs[0]).sort()).toEqual([
      'action', 'finishedAt', 'id', 'startedAt', 'status', 'steps', 'title',
    ]);
  });

  it('keeps the newest runs and enforces the serialized byte ceiling', () => {
    const runs = Array.from({ length: MAX_EXECUTION_HISTORY_RUNS + 5 }, (_, index) => run(index));
    const serialized = serializeExecutionHistory('interview-1', runs);

    expect(serialized).not.toBeNull();
    expect(new TextEncoder().encode(serialized!).byteLength).toBeLessThanOrEqual(MAX_EXECUTION_HISTORY_BYTES);
    const parsed = JSON.parse(serialized!);
    expect(parsed.runs).toHaveLength(MAX_EXECUTION_HISTORY_RUNS);
    expect(parsed.runs[0].id).toBe('run-5');
    expect(parsed.runs.at(-1).id).toBe(`run-${MAX_EXECUTION_HISTORY_RUNS + 4}`);
  });

  it('isolates histories by interview id and clears only the requested interview', () => {
    const storage = new MemoryStorage();
    writeExecutionHistory(storage, 'interview-1', [run(1)]);
    writeExecutionHistory(storage, 'interview-2', [run(2)]);

    expect(readExecutionHistory(storage, 'interview-1')[0].id).toBe('run-1');
    expect(readExecutionHistory(storage, 'interview-2')[0].id).toBe('run-2');
    clearExecutionHistory(storage, 'interview-1');
    expect(readExecutionHistory(storage, 'interview-1')).toEqual([]);
    expect(readExecutionHistory(storage, 'interview-2')).toHaveLength(1);
  });

  it('settles only the latest running answer from a durable recovery result', () => {
    const previous = run(1);
    const active = {
      ...run(2),
      status: 'running' as const,
      finishedAt: undefined,
      steps: [{ ...run(2).steps[0], status: 'running' as const }],
    };

    const settled = settleLatestRunningExecution(
      [previous, active],
      'answer',
      'completed',
      '2026-08-11T00:00:02.000Z',
    );

    expect(settled[0]).toEqual(previous);
    expect(settled[1]).toMatchObject({ status: 'completed', finishedAt: '2026-08-11T00:00:02.000Z' });
    expect(settled[1].steps[0]).toMatchObject({ status: 'completed', detail: '已从持久化会话确认提交结果。' });
  });
});
