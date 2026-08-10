'use client';

import { Activity, Check, ChevronDown, Clock3, Loader2, ShieldCheck, X } from 'lucide-react';
import type { ExecutionTraceStatus, InterviewExecutionRun } from '@/types/interview';

export function ExecutionTimeline({ runs }: { runs: InterviewExecutionRun[] }) {
  if (runs.length === 0) return null;

  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-card" aria-live="polite">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <Activity size={15} className="shrink-0 text-accent" />
          <h3 className="text-sm font-semibold text-primary">Agent 执行轨迹</h3>
          <span className="rounded bg-slate-100 px-2 py-0.5 text-[10px] font-medium text-slate-500">
            {runs.reduce((total, run) => total + run.steps.length, 0)} 个事件
          </span>
        </div>
        <span
          className="flex items-center gap-1.5 text-[10px] text-slate-400"
          title="记录可验证的执行步骤、状态、耗时与决策摘要，不包含模型私有推理文本"
        >
          <ShieldCheck size={12} className="text-emerald-500" />
          可审计轨迹
        </span>
      </div>

      <div className="divide-y divide-slate-100">
        {runs.map((run, index) => (
          <details key={run.id} open={run.status !== 'completed' || index === runs.length - 1} className="group">
            <summary className="flex min-h-12 cursor-pointer list-none items-center gap-3 px-4 py-3 hover:bg-slate-50 [&::-webkit-details-marker]:hidden">
              <RunStatusIcon status={run.status} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="text-xs font-semibold text-slate-700">{run.title}</span>
                  <span className={`text-[10px] font-semibold ${runStatusClass(run.status)}`}>{runStatusLabel(run.status)}</span>
                </div>
                <p className="mt-0.5 text-[10px] text-slate-400">
                  {formatTime(run.startedAt)} · {run.steps.length} 步{run.finishedAt ? ` · ${formatElapsed(run.startedAt, run.finishedAt)}` : ''}
                </p>
              </div>
              <ChevronDown size={14} className="shrink-0 text-slate-400 transition-transform group-open:rotate-180" />
            </summary>

            <ol className="border-t border-slate-100 px-4 py-2">
              {run.steps.map((step, stepIndex) => (
                <li key={step.id} className="grid grid-cols-[18px_minmax(0,1fr)_auto] gap-x-2 py-2">
                  <div className="relative flex justify-center">
                    {stepIndex < run.steps.length - 1 && <span className="absolute bottom-[-10px] top-4 w-px bg-slate-200" />}
                    <StepStatusIcon status={step.status} />
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="text-xs font-medium text-slate-700">{step.label}</span>
                      {step.agent && (
                        <span className="rounded bg-sky-50 px-1.5 py-0.5 font-mono text-[9px] text-sky-700">{agentLabel(step.agent)}</span>
                      )}
                    </div>
                    {step.detail && <p className="mt-0.5 text-[11px] leading-5 text-slate-500">{step.detail}</p>}
                    {step.transitions && step.transitions.length > 1 && (
                      <div className="mt-1.5 flex flex-wrap items-center gap-1 text-[9px] text-slate-400">
                        {step.transitions.map((transition, transitionIndex) => (
                          <span key={`${transition.status}-${transition.at}`} className="flex items-center gap-1">
                            {transitionIndex > 0 && <span className="text-slate-300">/</span>}
                            <span className={transitionClass(transition.status)}>{transitionLabel(transition.status)}</span>
                            <span className="tabular-nums">
                              {transition.durationMs != null && transition.durationMs > 0
                                ? formatDuration(transition.durationMs)
                                : formatTime(transition.at)}
                            </span>
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="pl-2 text-right text-[10px] tabular-nums text-slate-400">
                    {step.durationMs != null ? formatDuration(step.durationMs) : formatTime(step.at)}
                  </div>
                </li>
              ))}
            </ol>
          </details>
        ))}
      </div>
    </section>
  );
}

function RunStatusIcon({ status }: { status: InterviewExecutionRun['status'] }) {
  const className = 'flex h-7 w-7 shrink-0 items-center justify-center rounded-md';
  if (status === 'running') return <span className={`${className} bg-sky-50 text-sky-600`}><Loader2 size={14} className="animate-spin" /></span>;
  if (status === 'failed') return <span className={`${className} bg-red-50 text-red-600`}><X size={14} /></span>;
  return <span className={`${className} bg-emerald-50 text-emerald-600`}><Check size={14} /></span>;
}

function StepStatusIcon({ status }: { status: ExecutionTraceStatus }) {
  if (status === 'running') return <Loader2 size={13} className="z-10 mt-0.5 animate-spin bg-white text-sky-600" />;
  if (status === 'failed') return <X size={13} className="z-10 mt-0.5 bg-white text-red-600" />;
  if (status === 'completed') return <Check size={13} className="z-10 mt-0.5 bg-white text-emerald-600" />;
  return <Clock3 size={12} className="z-10 mt-0.5 bg-white text-slate-400" />;
}

function runStatusLabel(status: InterviewExecutionRun['status']) {
  return { running: '执行中', completed: '已完成', failed: '已停止' }[status];
}

function runStatusClass(status: InterviewExecutionRun['status']) {
  return { running: 'text-sky-700', completed: 'text-emerald-700', failed: 'text-red-700' }[status];
}

function transitionLabel(status: ExecutionTraceStatus) {
  return { queued: '排队', running: '开始', completed: '完成', failed: '失败' }[status];
}

function transitionClass(status: ExecutionTraceStatus) {
  return {
    queued: 'text-slate-500',
    running: 'text-sky-600',
    completed: 'text-emerald-600',
    failed: 'text-red-600',
  }[status];
}

function agentLabel(agent: string) {
  return {
    interviewer: 'Interviewer',
    assessor: 'Assessor',
    coverage_planner: 'Coverage Planner',
    reporter: 'Reporter',
  }[agent] ?? agent;
}

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--:--:--';
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date);
}

function formatElapsed(start: string, finish: string) {
  const duration = new Date(finish).getTime() - new Date(start).getTime();
  return formatDuration(Math.max(0, duration));
}

function formatDuration(durationMs: number) {
  if (durationMs < 1000) return `${Math.max(0, Math.round(durationMs))}ms`;
  return `${(durationMs / 1000).toFixed(durationMs >= 10000 ? 0 : 1)}s`;
}
