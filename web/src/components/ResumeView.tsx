'use client';

import { useState, useRef } from 'react';
import { Upload, FileText, CheckCircle, AlertTriangle, Star, ArrowRight, File, X, Globe, Type, Link2 } from 'lucide-react';

interface DiagnosisItem {
  section: string;
  score: number;
  issues: string[];
  suggestions: string[];
}

type InputMode = 'file' | 'text' | 'url';

const ACCEPTED_EXTENSIONS = ['.pdf', '.docx', '.doc', '.md', '.txt', '.tex'];
const ACCEPT_STRING = ACCEPTED_EXTENSIONS.join(',');

export function ResumeView() {
  const [content, setContent] = useState('');
  const [fileName, setFileName] = useState<string | null>(null);
  const [inputMode, setInputMode] = useState<InputMode>('file');
  const [urlInput, setUrlInput] = useState('');
  const [diagnosis, setDiagnosis] = useState<DiagnosisItem[] | null>(null);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [isParsing, setIsParsing] = useState(false);
  const [parseError, setParseError] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleAnalyze = async () => {
    if (!content.trim()) return;
    setIsAnalyzing(true);
    try {
      const res = await fetch('/api/resume', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content }),
      });
      const data = await res.json();
      if (data.diagnosis) {
        setDiagnosis(data.diagnosis);
      }
    } catch {
      setDiagnosis(null);
    } finally {
      setIsAnalyzing(false);
    }
  };

  const parseFile = async (file: globalThis.File) => {
    const ext = '.' + file.name.toLowerCase().split('.').pop();
    if (!ACCEPTED_EXTENSIONS.includes(ext)) {
      setParseError(`不支持的文件格式，请上传 ${ACCEPTED_EXTENSIONS.join(' / ')} 文件`);
      return;
    }

    setParseError(null);
    setFileName(file.name);
    setIsParsing(true);

    try {
      if (ext === '.txt' || ext === '.md') {
        const reader = new FileReader();
        reader.onload = (ev) => {
          setContent(ev.target?.result as string);
          setIsParsing(false);
        };
        reader.readAsText(file);
        return;
      }

      const formData = new FormData();
      formData.append('file', file);
      const res = await fetch('/api/parse-pdf', { method: 'POST', body: formData });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '文件解析失败');
      if (!data.text?.trim()) throw new Error('无法提取文本内容（可能是扫描件/图片文件）');
      setContent(data.text);
    } catch (err) {
      setParseError((err as Error).message);
      setFileName(null);
    } finally {
      setIsParsing(false);
    }
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = '';
    await parseFile(file);
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    const file = e.dataTransfer.files[0];
    if (!file) return;
    await parseFile(file);
  };

  const handleUrlFetch = async () => {
    if (!urlInput.trim()) return;
    setIsParsing(true);
    setParseError(null);
    try {
      const res = await fetch('/api/parse-url', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: urlInput }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '网页抓取失败');
      setContent(data.text);
      setFileName(urlInput);
    } catch (err) {
      setParseError((err as Error).message);
    } finally {
      setIsParsing(false);
    }
  };

  const clearAll = () => {
    setContent('');
    setFileName(null);
    setDiagnosis(null);
    setParseError(null);
    setUrlInput('');
  };

  const overallScore = diagnosis
    ? Math.round(diagnosis.reduce((sum, d) => sum + d.score, 0) / diagnosis.length * 10)
    : 0;

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-4xl space-y-6">
        {/* Input section */}
        <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-sm font-semibold text-primary">简历诊断</h3>
              <p className="text-xs text-slate-400 mt-0.5">多种方式导入简历，获取 AI 段落级诊断</p>
            </div>
            {content && (
              <button onClick={clearAll} className="flex items-center gap-1.5 rounded-lg border border-slate-200 px-3 py-1.5 text-xs text-slate-500 hover:border-red-200 hover:text-red-500 transition-all">
                <X size={12} />
                清除
              </button>
            )}
          </div>

          {/* Input mode tabs */}
          {!content && !isParsing && (
            <div className="flex gap-1 rounded-lg bg-slate-100 p-1 mb-4">
              {([
                { id: 'file' as const, label: '文件上传', icon: Upload },
                { id: 'text' as const, label: '粘贴文本', icon: Type },
                { id: 'url' as const, label: '网页导入', icon: Globe },
              ]).map((tab) => {
                const Icon = tab.icon;
                return (
                  <button
                    key={tab.id}
                    onClick={() => { setInputMode(tab.id); setParseError(null); }}
                    className={`flex-1 flex items-center justify-center gap-1.5 rounded-md px-3 py-2 text-xs font-medium transition-all ${
                      inputMode === tab.id
                        ? 'bg-white text-primary shadow-sm'
                        : 'text-slate-500 hover:text-slate-700'
                    }`}
                  >
                    <Icon size={13} />
                    {tab.label}
                  </button>
                );
              })}
            </div>
          )}

          {/* File drop zone */}
          {!content && !isParsing && inputMode === 'file' && (
            <div
              onDragOver={(e) => { e.preventDefault(); setIsDragging(true); }}
              onDragLeave={() => setIsDragging(false)}
              onDrop={handleDrop}
              onClick={() => fileInputRef.current?.click()}
              className={`flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed py-14 cursor-pointer transition-all ${
                isDragging
                  ? 'border-accent bg-accent/5 scale-[1.01]'
                  : 'border-slate-200 bg-surface-muted hover:border-accent/50 hover:bg-accent/5'
              }`}
            >
              <div className={`flex h-12 w-12 items-center justify-center rounded-xl transition-colors ${
                isDragging ? 'bg-accent/20' : 'bg-accent/10'
              }`}>
                <Upload size={22} className="text-accent" />
              </div>
              <div className="text-center">
                <p className="text-sm font-medium text-slate-600">
                  {isDragging ? '松开鼠标上传文件' : '拖拽简历到这里，或点击选择'}
                </p>
                <p className="text-xs text-slate-400 mt-1.5">
                  支持 PDF / DOCX / Markdown / TXT / LaTeX
                </p>
              </div>
              <input
                ref={fileInputRef}
                type="file"
                accept={ACCEPT_STRING}
                className="hidden"
                onChange={handleFileUpload}
              />
            </div>
          )}

          {/* Text input */}
          {!content && !isParsing && inputMode === 'text' && (
            <div>
              <textarea
                value={content}
                onChange={(e) => setContent(e.target.value)}
                placeholder="在此粘贴简历内容...&#10;&#10;支持 Markdown 格式，建议包含：&#10;- 个人信息&#10;- 项目经历（附技术栈和成果）&#10;- 技能清单&#10;- 教育背景"
                rows={12}
                autoFocus
                className="w-full rounded-xl border border-slate-200 bg-surface-muted px-4 py-3 text-sm text-slate-700 placeholder-slate-400 outline-none focus:border-accent/50 focus:ring-1 focus:ring-accent/20 resize-none transition-all"
              />
            </div>
          )}

          {/* URL input */}
          {!content && !isParsing && inputMode === 'url' && (
            <div className="space-y-3">
              <p className="text-xs text-slate-500">输入个人主页、LinkedIn、GitHub 或在线简历 URL</p>
              <div className="flex gap-2">
                <div className="flex-1 flex items-center gap-2 rounded-xl border border-slate-200 bg-surface-muted px-3">
                  <Link2 size={14} className="text-slate-400 shrink-0" />
                  <input
                    type="url"
                    value={urlInput}
                    onChange={(e) => setUrlInput(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleUrlFetch()}
                    placeholder="https://yoursite.com/resume 或 linkedin.com/in/yourname"
                    className="flex-1 bg-transparent py-3 text-sm text-slate-700 placeholder-slate-400 outline-none"
                    autoFocus
                  />
                </div>
                <button
                  onClick={handleUrlFetch}
                  disabled={!urlInput.trim()}
                  className="rounded-xl bg-accent px-4 py-2.5 text-sm font-medium text-white shadow-sm hover:bg-accent-dark disabled:opacity-50 transition-all"
                >
                  抓取
                </button>
              </div>
              <p className="text-[11px] text-slate-400">会提取网页正文内容用于分析，不会存储任何数据</p>
            </div>
          )}

          {/* Parsing state */}
          {isParsing && (
            <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-slate-200 bg-surface-muted py-12">
              <div className="h-8 w-8 rounded-full border-[3px] border-accent/30 border-t-accent animate-spin" />
              <p className="text-sm text-slate-500">正在解析...</p>
              <p className="text-xs text-slate-400">{fileName || urlInput}</p>
            </div>
          )}

          {/* Content loaded */}
          {content && !isParsing && (
            <div>
              {fileName && (
                <div className="flex items-center gap-2 mb-3 rounded-lg bg-accent/5 border border-accent/10 px-3 py-2">
                  <File size={14} className="text-accent" />
                  <span className="text-xs font-medium text-accent-dark truncate">{fileName}</span>
                  <span className="text-[11px] text-slate-400 ml-auto shrink-0">{content.length} 字</span>
                </div>
              )}
              <textarea
                value={content}
                onChange={(e) => setContent(e.target.value)}
                rows={10}
                className="w-full rounded-xl border border-slate-200 bg-surface-muted px-4 py-3 text-sm text-slate-700 placeholder-slate-400 outline-none focus:border-accent/50 focus:ring-1 focus:ring-accent/20 resize-none transition-all"
              />
            </div>
          )}

          {/* Error */}
          {parseError && (
            <div className="mt-3 flex items-center gap-2 rounded-lg bg-red-50 border border-red-100 px-3 py-2">
              <AlertTriangle size={13} className="text-red-400 shrink-0" />
              <span className="text-xs text-red-600">{parseError}</span>
            </div>
          )}

          {/* Action row */}
          {content && (
            <div className="flex items-center justify-between mt-4">
              <label className="flex items-center gap-2 rounded-lg border border-slate-200 px-3 py-2 text-xs text-slate-600 hover:border-accent hover:text-accent cursor-pointer transition-all">
                <Upload size={13} />
                重新上传
                <input type="file" accept={ACCEPT_STRING} className="hidden" onChange={handleFileUpload} />
              </label>
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
          )}
        </div>

        {/* Results */}
        {diagnosis && (
          <div className="space-y-4 animate-slide-up">
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
                  {item.issues.length > 0 && (
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
                  )}

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
