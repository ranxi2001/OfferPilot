'use client';

import { useState } from 'react';
import { Upload, FileText, CheckCircle, AlertTriangle, Star, ArrowRight } from 'lucide-react';

interface DiagnosisItem {
  section: string;
  score: number;
  issues: string[];
  suggestions: string[];
}

const MOCK_DIAGNOSIS: DiagnosisItem[] = [
  {
    section: '项目经历 #1',
    score: 7,
    issues: ['缺少量化数据', '未体现技术选型决策'],
    suggestions: ['加入性能指标：延迟、QPS、成功率', '描述为什么选择该技术方案'],
  },
  {
    section: '项目经历 #2',
    score: 5,
    issues: ['描述偏笼统', '缺少 STAR 结构', '未提及个人贡献'],
    suggestions: ['用 Situation-Task-Action-Result 结构重写', '突出个人在团队中的角色', '加入具体技术栈'],
  },
  {
    section: '技能清单',
    score: 8,
    issues: ['部分技能缺乏经验佐证'],
    suggestions: ['对重点技能标注熟练程度', '与项目经历对应匹配'],
  },
];

export function ResumeView() {
  const [content, setContent] = useState('');
  const [diagnosis, setDiagnosis] = useState<DiagnosisItem[] | null>(null);
  const [isAnalyzing, setIsAnalyzing] = useState(false);

  const handleAnalyze = async () => {
    if (!content.trim()) return;
    setIsAnalyzing(true);
    await new Promise((r) => setTimeout(r, 1500));
    setDiagnosis(MOCK_DIAGNOSIS);
    setIsAnalyzing(false);
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (ev) => {
      setContent(ev.target?.result as string);
    };
    reader.readAsText(file);
    e.target.value = '';
  };

  const overallScore = diagnosis
    ? Math.round(diagnosis.reduce((sum, d) => sum + d.score, 0) / diagnosis.length * 10)
    : 0;

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-4xl space-y-6">
        {/* Upload section */}
        <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-sm font-semibold text-primary">简历内容</h3>
              <p className="text-xs text-slate-400 mt-0.5">粘贴简历文本或上传文件，获取段落级诊断</p>
            </div>
            <label className="flex items-center gap-2 rounded-lg border border-slate-200 px-3 py-2 text-xs text-slate-600 hover:border-accent hover:text-accent cursor-pointer transition-all">
              <Upload size={13} />
              上传文件
              <input type="file" accept=".txt,.md,.pdf" className="hidden" onChange={handleFileUpload} />
            </label>
          </div>

          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder="在此粘贴你的简历内容...&#10;&#10;支持 Markdown 格式。建议包含：&#10;- 项目经历（附技术栈和成果）&#10;- 技能清单&#10;- 教育背景"
            rows={8}
            className="w-full rounded-xl border border-slate-200 bg-surface-muted px-4 py-3 text-sm text-slate-700 placeholder-slate-400 outline-none focus:border-accent/50 focus:ring-1 focus:ring-accent/20 resize-none transition-all"
          />

          <div className="flex items-center justify-between mt-4">
            <span className="text-[11px] text-slate-400">
              {content.length > 0 ? `${content.length} 字` : ''}
            </span>
            <button
              onClick={handleAnalyze}
              disabled={!content.trim() || isAnalyzing}
              className="flex items-center gap-2 rounded-xl bg-accent px-5 py-2.5 text-sm font-medium text-white shadow-sm hover:bg-accent-dark disabled:opacity-50 transition-all"
            >
              {isAnalyzing ? (
                <>
                  <div className="h-3.5 w-3.5 rounded-full border-2 border-white/30 border-t-white animate-spin" />
                  分析中...
                </>
              ) : (
                <>
                  <Star size={14} />
                  开始诊断
                </>
              )}
            </button>
          </div>
        </div>

        {/* Results */}
        {diagnosis && (
          <div className="space-y-4 animate-slide-up">
            {/* Overall score */}
            <div className="rounded-2xl border border-accent/20 bg-gradient-to-r from-accent/5 to-cyan/5 p-5 shadow-card">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-semibold text-primary">诊断完成</h3>
                  <p className="text-xs text-slate-500 mt-0.5">共分析 {diagnosis.length} 个段落</p>
                </div>
                <div className="flex items-center gap-3">
                  <div className="text-right">
                    <div className="text-2xl font-bold text-accent">{overallScore}</div>
                    <div className="text-[11px] text-slate-400">/ 100</div>
                  </div>
                  <div className="h-12 w-12 rounded-full border-4 border-accent/20 flex items-center justify-center">
                    <CheckCircle size={20} className="text-accent" />
                  </div>
                </div>
              </div>
            </div>

            {/* Section results */}
            {diagnosis.map((item, i) => (
              <div key={i} className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <FileText size={14} className="text-slate-400" />
                    <span className="text-sm font-medium text-primary">{item.section}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 w-16 rounded-full bg-slate-100 overflow-hidden">
                      <div
                        className={`h-full rounded-full ${
                          item.score >= 8 ? 'bg-emerald-400' : item.score >= 6 ? 'bg-accent' : 'bg-amber-400'
                        }`}
                        style={{ width: `${item.score * 10}%` }}
                      />
                    </div>
                    <span className={`text-xs font-semibold ${
                      item.score >= 8 ? 'text-emerald-500' : item.score >= 6 ? 'text-accent' : 'text-amber-500'
                    }`}>
                      {item.score}/10
                    </span>
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  {/* Issues */}
                  <div className="rounded-xl bg-amber-50/50 border border-amber-100 p-3">
                    <div className="flex items-center gap-1.5 mb-2">
                      <AlertTriangle size={11} className="text-amber-500" />
                      <span className="text-[11px] font-medium text-amber-700">问题</span>
                    </div>
                    <ul className="space-y-1">
                      {item.issues.map((issue, j) => (
                        <li key={j} className="text-[12px] text-amber-800 leading-relaxed flex gap-1.5">
                          <span className="text-amber-400 mt-0.5">-</span>
                          {issue}
                        </li>
                      ))}
                    </ul>
                  </div>

                  {/* Suggestions */}
                  <div className="rounded-xl bg-accent/5 border border-accent/10 p-3">
                    <div className="flex items-center gap-1.5 mb-2">
                      <ArrowRight size={11} className="text-accent" />
                      <span className="text-[11px] font-medium text-accent-dark">建议</span>
                    </div>
                    <ul className="space-y-1">
                      {item.suggestions.map((sug, j) => (
                        <li key={j} className="text-[12px] text-slate-600 leading-relaxed flex gap-1.5">
                          <span className="text-accent mt-0.5">-</span>
                          {sug}
                        </li>
                      ))}
                    </ul>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
