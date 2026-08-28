'use client';

import { useState } from 'react';
import { AlertCircle, CheckCircle2, XCircle, ArrowRight, Sparkles } from 'lucide-react';
import { MaterialInput } from '@/components/MaterialInput';
import type { InterviewMaterial } from '@/types/interview';

interface MatchResult {
  score: number;
  matched: string[];
  missing: string[];
  suggestions: string[];
  level: string;
  focus: string[];
  summary: string;
  breakdown: {
    mustHave: number;
    responsibilities: number;
    evidenceQuality: number;
    bonus: number;
  };
}

const scoreDimensions = [
  ['mustHave', '硬性要求', 45],
  ['responsibilities', '职责匹配', 25],
  ['evidenceQuality', '履历证据', 20],
  ['bonus', '加分项', 10],
] as const;

export function MatchView() {
  const [jd, setJd] = useState<InterviewMaterial | null>(null);
  const [resume, setResume] = useState<InterviewMaterial | null>(null);
  const [result, setResult] = useState<MatchResult | null>(null);
  const [isMatching, setIsMatching] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleMatch = async () => {
    if (!jd?.text.trim() || !resume?.text.trim()) return;
    setIsMatching(true);
    setError(null);
    setResult(null);
    try {
      const res = await fetch('/api/match', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ jd: jd.text, resume: resume.text }),
      });
      const data = await res.json() as MatchResult & {
        error?: string | { message?: string };
      };
      if (!res.ok || data.score === undefined) {
        const message = typeof data.error === 'string' ? data.error : data.error?.message;
        throw new Error(message || '语义匹配失败');
      }
      setResult(data);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setIsMatching(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-5xl space-y-6">
        <div className="grid gap-4 lg:grid-cols-2">
          <MaterialInput
            label="职位描述（JD）"
            description="岗位要求、职责与加分项"
            emptyName="粘贴的职位描述"
            value={jd}
            onChange={setJd}
          />
          <MaterialInput
            label="候选人简历"
            description="项目经历、个人贡献与量化结果"
            emptyName="粘贴的简历"
            value={resume}
            onChange={setResume}
          />
        </div>

        {/* Action */}
        <div className="flex justify-center">
          <button
            onClick={handleMatch}
            disabled={!jd?.text.trim() || !resume?.text.trim() || isMatching}
            className="flex items-center gap-2 rounded-xl bg-gradient-to-r from-accent to-cyan px-8 py-3 text-sm font-medium text-white shadow-elevated hover:shadow-glow disabled:opacity-50 transition-all"
          >
            {isMatching ? (
              <>
                <div className="h-4 w-4 rounded-full border-2 border-white/30 border-t-white animate-spin" />
                匹配分析中...
              </>
            ) : (
              <>
                <Sparkles size={15} />
                开始匹配分析
              </>
            )}
          </button>
        </div>

        {error && (
          <div className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            <AlertCircle size={16} className="mt-0.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {/* Results */}
        {result && (
          <div className="space-y-4 animate-slide-up">
            {/* Score card */}
            <div className="rounded-2xl border border-accent/20 bg-gradient-to-r from-white to-accent/5 p-6 shadow-card">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-semibold text-primary mb-1">匹配度分析</h3>
                  <div className="flex items-center gap-2">
                    <span className="rounded-full bg-accent/10 px-2.5 py-0.5 text-xs font-medium text-accent">
                      {result.level}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-1.5 mt-3">
                    {result.focus.map((f) => (
                      <span key={f} className="rounded-md bg-navy-50 px-2 py-0.5 text-[11px] text-navy-700">
                        {f}
                      </span>
                    ))}
                  </div>
                </div>
                <div className="relative">
                  <svg width="80" height="80" viewBox="0 0 80 80">
                    <circle cx="40" cy="40" r="34" fill="none" stroke="#e2e8f0" strokeWidth="6" />
                    <circle
                      cx="40" cy="40" r="34"
                      fill="none"
                      stroke="#0ea5e9"
                      strokeWidth="6"
                      strokeLinecap="round"
                      strokeDasharray={`${(result.score / 100) * 213.6} 213.6`}
                      transform="rotate(-90 40 40)"
                    />
                  </svg>
                  <div className="absolute inset-0 flex flex-col items-center justify-center">
                    <span className="text-xl font-bold text-accent">{result.score}%</span>
                  </div>
                </div>
              </div>
              <p className="mt-4 border-t border-slate-100 pt-4 text-sm leading-6 text-slate-600">
                {result.summary}
              </p>
              <div className="mt-4 grid grid-cols-2 gap-x-5 gap-y-3 sm:grid-cols-4">
                {scoreDimensions.map(([key, label, maximum]) => (
                  <div key={key} className="min-w-0">
                    <div className="mb-1.5 flex items-center justify-between gap-2 text-[11px] text-slate-500">
                      <span>{label}</span>
                      <span className="tabular-nums text-slate-700">{result.breakdown[key]}/{maximum}</span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-slate-100">
                      <div
                        className="h-full rounded-full bg-accent"
                        style={{ width: `${Math.min(100, (result.breakdown[key] / maximum) * 100)}%` }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Matched vs Missing */}
            <div className="grid gap-4 sm:grid-cols-2">
              {/* Matched */}
              <div className="rounded-2xl border border-emerald-200/80 bg-white p-5 shadow-card">
                <div className="flex items-center gap-2 mb-3">
                  <CheckCircle2 size={14} className="text-emerald-500" />
                  <h4 className="text-sm font-medium text-emerald-700">已验证匹配</h4>
                  <span className="text-[11px] text-emerald-400 ml-auto">{result.matched.length} 项</span>
                </div>
                <div className="flex flex-wrap gap-2">
                  {result.matched.map((kw) => (
                    <span key={kw} className="rounded-lg bg-emerald-50 border border-emerald-100 px-2.5 py-1 text-xs text-emerald-700">
                      {kw}
                    </span>
                  ))}
                </div>
              </div>

              {/* Missing */}
              <div className="rounded-2xl border border-red-200/80 bg-white p-5 shadow-card">
                <div className="flex items-center gap-2 mb-3">
                  <XCircle size={14} className="text-red-400" />
                  <h4 className="text-sm font-medium text-red-600">关键差距</h4>
                  <span className="text-[11px] text-red-300 ml-auto">{result.missing.length} 项</span>
                </div>
                <div className="flex flex-wrap gap-2">
                  {result.missing.map((kw) => (
                    <span key={kw} className="rounded-lg bg-red-50 border border-red-100 px-2.5 py-1 text-xs text-red-600">
                      {kw}
                    </span>
                  ))}
                </div>
              </div>
            </div>

            {/* Suggestions */}
            <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
              <div className="flex items-center gap-2 mb-3">
                <ArrowRight size={14} className="text-accent" />
                <h4 className="text-sm font-medium text-primary">定向包装建议</h4>
              </div>
              <div className="space-y-2.5">
                {result.suggestions.map((sug, i) => (
                  <div key={i} className="flex gap-3 rounded-xl bg-surface-muted p-3">
                    <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent/10 text-[11px] font-bold text-accent">
                      {i + 1}
                    </span>
                    <p className="text-xs text-slate-600 leading-relaxed">{sug}</p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
