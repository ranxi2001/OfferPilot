import type {
  ExecutionTraceStatus,
  InterviewAction,
  InterviewExecutionRun,
  InterviewExecutionTrace,
  InterviewExecutionTransition,
} from '@/types/interview';

export const EXECUTION_HISTORY_KEY_PREFIX = 'offerpilot.interview.execution-runs.v1';
export const MAX_EXECUTION_HISTORY_BYTES = 256 * 1024;
export const MAX_EXECUTION_HISTORY_RUNS = 64;
export const MAX_EXECUTION_STEPS_PER_RUN = 64;
export const MAX_EXECUTION_TRANSITIONS_PER_STEP = 16;

export interface ExecutionHistoryStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

interface ExecutionHistoryEnvelope {
  version: 1;
  interviewId: string;
  runs: InterviewExecutionRun[];
}

const actionValues = new Set<InterviewAction>(['start', 'answer', 'report']);
const traceStatusValues = new Set<ExecutionTraceStatus>(['queued', 'running', 'completed', 'failed']);
const runStatusValues = new Set<InterviewExecutionRun['status']>(['running', 'completed', 'failed']);

export function sanitizeExecutionRuns(value: unknown): InterviewExecutionRun[] {
  if (!Array.isArray(value)) return [];
  return value
    .slice(-MAX_EXECUTION_HISTORY_RUNS)
    .map(sanitizeRun)
    .filter((run): run is InterviewExecutionRun => run !== null);
}

export function serializeExecutionHistory(
  interviewId: string,
  value: unknown,
  maxBytes = MAX_EXECUTION_HISTORY_BYTES,
): string | null {
  const normalizedInterviewId = boundedString(interviewId, 200);
  if (!normalizedInterviewId || maxBytes <= 0) return null;

  let runs = sanitizeExecutionRuns(value);
  if (runs.length === 0) return null;

  while (runs.length > 0) {
    const serialized = JSON.stringify({
      version: 1,
      interviewId: normalizedInterviewId,
      runs,
    } satisfies ExecutionHistoryEnvelope);
    if (new TextEncoder().encode(serialized).byteLength <= maxBytes) return serialized;

    if (runs.length > 1) {
      runs = runs.slice(1);
      continue;
    }
    if (runs[0].steps.length > 1) {
      runs = [{ ...runs[0], steps: runs[0].steps.slice(1) }];
      continue;
    }
    return null;
  }
  return null;
}

export function readExecutionHistory(
  storage: ExecutionHistoryStorage | null | undefined,
  interviewId: string,
): InterviewExecutionRun[] {
  if (!storage) return [];
  const key = executionHistoryKey(interviewId);
  if (!key) return [];

  try {
    const serialized = storage.getItem(key);
    if (!serialized || new TextEncoder().encode(serialized).byteLength > MAX_EXECUTION_HISTORY_BYTES) return [];
    const envelope = JSON.parse(serialized) as Partial<ExecutionHistoryEnvelope>;
    if (envelope.version !== 1 || envelope.interviewId !== interviewId) return [];
    return sanitizeExecutionRuns(envelope.runs);
  } catch {
    return [];
  }
}

export function writeExecutionHistory(
  storage: ExecutionHistoryStorage | null | undefined,
  interviewId: string,
  runs: InterviewExecutionRun[],
): void {
  if (!storage) return;
  const key = executionHistoryKey(interviewId);
  if (!key) return;

  try {
    const serialized = serializeExecutionHistory(interviewId, runs);
    if (serialized) storage.setItem(key, serialized);
    else storage.removeItem(key);
  } catch {
    // Interview execution remains usable when browser storage is unavailable.
  }
}

export function clearExecutionHistory(
  storage: ExecutionHistoryStorage | null | undefined,
  interviewId: string,
): void {
  const key = executionHistoryKey(interviewId);
  if (!storage || !key) return;
  try {
    storage.removeItem(key);
  } catch {
    // Reset remains usable when browser storage is unavailable.
  }
}

export function settleLatestRunningExecution(
  value: InterviewExecutionRun[],
  action: InterviewAction,
  outcome: 'completed' | 'failed',
  finishedAt: string,
): InterviewExecutionRun[] {
  const runs = sanitizeExecutionRuns(value);
  const position = runs.findLastIndex((run) => run.action === action && run.status === 'running');
  if (position < 0) return runs;

  const detail = outcome === 'completed'
    ? '已从持久化会话确认提交结果。'
    : '持久化事件确认本次执行未提交。';
  const run = runs[position];
  let settledStep = false;
  const steps = run.steps.map((step) => {
    if (step.status !== 'queued' && step.status !== 'running') return step;
    settledStep = true;
    const transition: InterviewExecutionTransition = { status: outcome, at: finishedAt, detail };
    return {
      ...step,
      status: outcome,
      at: finishedAt,
      detail,
      transitions: [...(step.transitions ?? []), transition],
    };
  });
  if (!settledStep) {
    steps.push({
      id: `${run.id}:recovered`,
      stage: 'recovery',
      label: outcome === 'completed' ? '提交结果已恢复' : '失败状态已恢复',
      detail,
      status: outcome,
      at: finishedAt,
      transitions: [{ status: outcome, at: finishedAt, detail }],
    });
  }
  runs[position] = { ...run, status: outcome, finishedAt, steps };
  return runs;
}

function executionHistoryKey(interviewId: string): string | null {
  const normalized = boundedString(interviewId, 200);
  return normalized ? `${EXECUTION_HISTORY_KEY_PREFIX}:${encodeURIComponent(normalized)}` : null;
}

function sanitizeRun(value: unknown): InterviewExecutionRun | null {
  if (!isRecord(value)) return null;
  const id = boundedString(value.id, 180);
  const action = actionValues.has(value.action as InterviewAction) ? value.action as InterviewAction : null;
  const title = boundedString(value.title, 240);
  const status = runStatusValues.has(value.status as InterviewExecutionRun['status'])
    ? value.status as InterviewExecutionRun['status']
    : null;
  const startedAt = boundedString(value.startedAt, 64);
  if (!id || !action || !title || !status || !startedAt) return null;

  const finishedAt = boundedString(value.finishedAt, 64);
  const steps = Array.isArray(value.steps)
    ? value.steps.slice(-MAX_EXECUTION_STEPS_PER_RUN).map(sanitizeTrace).filter((step): step is InterviewExecutionTrace => step !== null)
    : [];
  return {
    id,
    action,
    title,
    status,
    startedAt,
    ...(finishedAt ? { finishedAt } : {}),
    steps,
  };
}

function sanitizeTrace(value: unknown): InterviewExecutionTrace | null {
  if (!isRecord(value)) return null;
  const id = boundedString(value.id, 180);
  const stage = boundedString(value.stage, 80);
  const label = boundedString(value.label, 240);
  const status = traceStatusValues.has(value.status as ExecutionTraceStatus)
    ? value.status as ExecutionTraceStatus
    : null;
  const at = boundedString(value.at, 64);
  if (!id || !stage || !label || !status || !at) return null;

  const detail = boundedString(value.detail, 800);
  const agent = boundedString(value.agent, 80);
  const durationMs = boundedDuration(value.durationMs);
  const transitions = Array.isArray(value.transitions)
    ? value.transitions
      .slice(-MAX_EXECUTION_TRANSITIONS_PER_STEP)
      .map(sanitizeTransition)
      .filter((transition): transition is InterviewExecutionTransition => transition !== null)
    : [];
  return {
    id,
    stage,
    label,
    status,
    at,
    ...(detail ? { detail } : {}),
    ...(agent ? { agent } : {}),
    ...(durationMs !== undefined ? { durationMs } : {}),
    ...(transitions.length > 0 ? { transitions } : {}),
  };
}

function sanitizeTransition(value: unknown): InterviewExecutionTransition | null {
  if (!isRecord(value)) return null;
  const status = traceStatusValues.has(value.status as ExecutionTraceStatus)
    ? value.status as ExecutionTraceStatus
    : null;
  const at = boundedString(value.at, 64);
  if (!status || !at) return null;
  const detail = boundedString(value.detail, 800);
  const durationMs = boundedDuration(value.durationMs);
  return {
    status,
    at,
    ...(detail ? { detail } : {}),
    ...(durationMs !== undefined ? { durationMs } : {}),
  };
}

function boundedString(value: unknown, maxLength: number): string | null {
  if (typeof value !== 'string') return null;
  const normalized = value.trim();
  return normalized ? normalized.slice(0, maxLength) : null;
}

function boundedDuration(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return undefined;
  return Math.min(Math.round(value), 24 * 60 * 60 * 1000);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
