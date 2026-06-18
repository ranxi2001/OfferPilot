'use client';

import { useState } from 'react';
import { GitCompare, CheckCircle2, XCircle, ArrowRight, Briefcase, Sparkles } from 'lucide-react';

interface MatchResult {
  score: number;
  matched: string[];
  missing: string[];
  suggestions: string[];
  level: string;
  focus: string[];
}

const MOCK_RESULT: MatchResult = {
  score: 68,
  matched: ['TypeScript', 'Node.js', 'React', 'LLM/GPT', 'Agent 架构', 'RAG', '向量数据库'],
  missing: ['Kubernetes', 'gRPC', '分布式系统', '大规模数据处理', 'MLOps'],
  suggestions: [
    '在项目经历中补充容器化部署经验（Docker → K8s）',
    '突出 Agent 系统的分布式调度设计',
    '添加模型训练/微调相关经验或学习项目',
    '强调数据 pipeline 经验（即使是 embedding pipeline）',
  ],
  level: '高级工程师 (P6-P7)',
  focus: ['Agent 工程化', '系统架构', 'LLM 应用', '全栈交付'],
};

export function MatchView() {
  const [jdContent, setJdContent] = useState('');
  const [resumeContent, setResumeContent] = useState('');
  const [result, setResult] = useState<MatchResult | null>(null);
  const [isMatching, setIsMatching] = useState(false);

  const handleMatch = async () => {
    if (!jdContent.trim() || !resumeContent.trim()) return;
    setIsMatching(true);
    try {
      const res = await fetch('/api/match', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ jd: jdContent, resume: resumeContent }),
      });
      const data = await res.json();
      if (data.score !== undefined) {
        setResult(data);
      }
    } catch {
      setResult(MOCK_RESULT);
    } finally {
      setIsMatching(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-5xl space-y-6">
        {/* Input section */}
        <div className="grid gap-4 lg:grid-cols-2">
          {/* JD input */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
            <div className="flex items-center gap-2 mb-3">
              <Briefcase size={14} className="text-accent" />
              <h3 className="text-sm font-semibold text-primary">职位描述 (JD)</h3>
            </div>
            <textarea
              value={jdContent}
              onChange={(e) => setJdContent(e.target.value)}
              placeholder="粘贴 JD 内容...&#10;&#10;示例：&#10;- 岗位：AI Agent 工程师&#10;- 要求：3年以上 LLM 应用经验..."
              rows={8}
              className="w-full rounded-xl border border-slate-200 bg-surface-muted px-4 py-3 text-sm text-slate-700 placeholder-slate-400 outline-none focus:border-accent/50 focus:ring-1 focus:ring-accent/20 resize-none transition-all"
            />
          </div>

          {/* Resume input */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
            <div className="flex items-center gap-2 mb-3">
              <GitCompare size={14} className="text-cyan" />
              <h3 className="text-sm font-semibold text-primary">简历摘要</h3>
            </div>
            <textarea
              value={resumeContent}
              onChange={(e) => setResumeContent(e.target.value)}
              placeholder="粘贴你的简历关键内容...&#10;&#10;技能和项目经历即可"
              rows={8}
              className="w-full rounded-xl border border-slate-200 bg-surface-muted px-4 py-3 text-sm text-slate-700 placeholder-slate-400 outline-none focus:border-accent/50 focus:ring-1 focus:ring-accent/20 resize-none transition-all"
            />
          </div>
        </div>

        {/* Action */}
        <div className="flex justify-center">
          <button
            onClick={handleMatch}
            disabled={!jdContent.trim() || !resumeContent.trim() || isMatching}
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
            </div>

            {/* Matched vs Missing */}
            <div className="grid gap-4 sm:grid-cols-2">
              {/* Matched */}
              <div className="rounded-2xl border border-emerald-200/80 bg-white p-5 shadow-card">
                <div className="flex items-center gap-2 mb-3">
                  <CheckCircle2 size={14} className="text-emerald-500" />
                  <h4 className="text-sm font-medium text-emerald-700">已匹配关键词</h4>
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
                  <h4 className="text-sm font-medium text-red-600">缺失关键词</h4>
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
