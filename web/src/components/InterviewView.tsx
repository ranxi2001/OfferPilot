'use client';

import { useState, useRef } from 'react';
import { Mic, Square, Play, ArrowRight, BarChart3, AlertTriangle, CheckCircle, XCircle, Volume2 } from 'lucide-react';

type Phase = 'setup' | 'questioning' | 'feedback' | 'report';

interface Defect {
  id: string;
  type: string;
  severity: 'minor' | 'moderate' | 'critical';
  description: string;
  suggestion: string;
}

interface QuestionFeedback {
  question: string;
  answer: string;
  defects: Defect[];
  summary: string;
}

interface Report {
  totalQuestions: number;
  totalDefects: number;
  bySeverity: { critical: number; moderate: number; minor: number };
  topIssues: { type: string; count: number }[];
  overallScore: number;
}

export function InterviewView() {
  const [phase, setPhase] = useState<Phase>('setup');
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [currentQuestion, setCurrentQuestion] = useState('');
  const [progress, setProgress] = useState({ current: 0, total: 5 });
  const [isRecording, setIsRecording] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [textAnswer, setTextAnswer] = useState('');
  const [currentFeedback, setCurrentFeedback] = useState<QuestionFeedback | null>(null);
  const [hasNext, setHasNext] = useState(true);
  const [history, setHistory] = useState<QuestionFeedback[]>([]);
  const [report, setReport] = useState<Report | null>(null);
  const [isSpeaking, setIsSpeaking] = useState(false);

  const audioContextRef = useRef<AudioContext | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const samplesRef = useRef<Float32Array[]>([]);
  const sampleRateRef = useRef(16000);

  async function startInterview() {
    const res = await fetch('/api/interview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'start' }),
    });
    const data = await res.json();
    setSessionId(data.sessionId);
    setCurrentQuestion(data.question);
    setProgress(data.progress);
    setPhase('questioning');
    speakQuestion(data.question);
  }

  function speakQuestion(text: string) {
    if ('speechSynthesis' in window) {
      window.speechSynthesis.cancel();
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.lang = 'zh-CN';
      utterance.rate = 0.9;
      utterance.onstart = () => setIsSpeaking(true);
      utterance.onend = () => setIsSpeaking(false);
      window.speechSynthesis.speak(utterance);
    }
  }

  async function submitAnswer(answerText: string) {
    if (!answerText.trim() || isSubmitting) return;
    setIsSubmitting(true);

    const res = await fetch('/api/interview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'answer', sessionId, answer: answerText }),
    });
    const data = await res.json();

    const feedback: QuestionFeedback = {
      question: currentQuestion,
      answer: answerText,
      defects: data.defects,
      summary: data.summary,
    };
    setCurrentFeedback(feedback);
    setHistory((prev) => [...prev, feedback]);
    setHasNext(data.hasNext);
    setProgress(data.progress);
    setPhase('feedback');
    setTextAnswer('');
    setIsSubmitting(false);
  }

  async function nextQuestion() {
    const res = await fetch('/api/interview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'next', sessionId }),
    });
    const data = await res.json();
    setCurrentQuestion(data.question);
    setProgress(data.progress);
    setCurrentFeedback(null);
    setPhase('questioning');
    speakQuestion(data.question);
  }

  async function showReport() {
    const res = await fetch('/api/interview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'report', sessionId }),
    });
    const data = await res.json();
    setReport(data);
    setPhase('report');
  }

  async function startRecording() {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const AudioContextCtor = window.AudioContext || (window as any).webkitAudioContext;
    const audioContext = new AudioContextCtor();
    const source = audioContext.createMediaStreamSource(stream);
    const processor = audioContext.createScriptProcessor(4096, 1, 1);

    samplesRef.current = [];
    sampleRateRef.current = audioContext.sampleRate;
    processor.onaudioprocess = (event) => {
      samplesRef.current.push(new Float32Array(event.inputBuffer.getChannelData(0)));
    };

    source.connect(processor);
    processor.connect(audioContext.destination);
    streamRef.current = stream;
    audioContextRef.current = audioContext;
    processorRef.current = processor;
    setIsRecording(true);
  }

  async function stopRecording() {
    processorRef.current?.disconnect();
    streamRef.current?.getTracks().forEach((t) => t.stop());
    void audioContextRef.current?.close();
    processorRef.current = null;
    streamRef.current = null;
    audioContextRef.current = null;
    setIsRecording(false);

    const wav = encodeWav(samplesRef.current, sampleRateRef.current);

    try {
      const res = await fetch('/api/transcribe', {
        method: 'POST',
        headers: { 'Content-Type': 'audio/wav', 'X-File-Name': 'interview.wav' },
        body: wav,
      });
      const data = await res.json();
      if (data.text) {
        await submitAnswer(data.text);
      }
    } catch {
      // Fallback: if transcribe fails, user can type
    }
  }

  function resetInterview() {
    setPhase('setup');
    setSessionId(null);
    setCurrentQuestion('');
    setCurrentFeedback(null);
    setHistory([]);
    setReport(null);
    setHasNext(true);
  }

  // Setup phase
  if (phase === 'setup') {
    return (
      <div className="flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto max-w-2xl">
          <div className="rounded-2xl border border-slate-200/80 bg-white p-8 shadow-card text-center">
            <div className="mb-6 flex justify-center">
              <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-accent to-cyan shadow-elevated">
                <Mic size={28} className="text-white" />
              </div>
            </div>
            <h2 className="text-xl font-bold text-primary mb-2">实时面试模拟</h2>
            <p className="text-sm text-slate-500 mb-6 max-w-md mx-auto">
              AI 面试官逐题提问，支持语音/文字作答。每题即时反馈缺陷，结束后生成总结报告。
            </p>

            <div className="rounded-xl bg-surface-muted border border-slate-200 p-4 mb-6 text-left">
              <h4 className="text-xs font-semibold text-slate-600 mb-2">面试流程</h4>
              <div className="space-y-2 text-xs text-slate-500">
                <div className="flex items-center gap-2">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-accent/10 text-[10px] font-bold text-accent">1</span>
                  AI 面试官语音播报问题
                </div>
                <div className="flex items-center gap-2">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-accent/10 text-[10px] font-bold text-accent">2</span>
                  你录音或打字作答
                </div>
                <div className="flex items-center gap-2">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-accent/10 text-[10px] font-bold text-accent">3</span>
                  即时缺陷分析 + 改进建议
                </div>
                <div className="flex items-center gap-2">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-accent/10 text-[10px] font-bold text-accent">4</span>
                  面试结束后生成总结报告
                </div>
              </div>
            </div>

            <button
              onClick={startInterview}
              className="flex items-center gap-2 mx-auto rounded-xl bg-gradient-to-r from-accent to-cyan px-8 py-3 text-sm font-medium text-white shadow-elevated hover:shadow-glow transition-all"
            >
              <Play size={15} />
              开始面试
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Questioning phase
  if (phase === 'questioning') {
    return (
      <div className="flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto max-w-2xl space-y-4">
          {/* Progress */}
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-400">第 {progress.current} / {progress.total} 题</span>
            <div className="h-1.5 flex-1 mx-4 rounded-full bg-slate-100 overflow-hidden">
              <div
                className="h-full rounded-full bg-gradient-to-r from-accent to-cyan transition-all"
                style={{ width: `${(progress.current / progress.total) * 100}%` }}
              />
            </div>
          </div>

          {/* Question card */}
          <div className="rounded-2xl border border-accent/20 bg-white p-6 shadow-card">
            <div className="flex items-center gap-2 mb-3">
              <Volume2 size={14} className={isSpeaking ? 'text-accent animate-pulse' : 'text-slate-400'} />
              <span className="text-xs font-medium text-slate-500">面试官提问</span>
            </div>
            <p className="text-base font-medium text-primary leading-relaxed">{currentQuestion}</p>
            {!isSpeaking && (
              <button
                onClick={() => speakQuestion(currentQuestion)}
                className="mt-3 text-[11px] text-accent hover:underline"
              >
                重新播放语音
              </button>
            )}
          </div>

          {/* Answer input */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
            <div className="flex items-center gap-2 mb-3">
              <span className="text-xs font-medium text-slate-500">你的回答</span>
              <span className="text-[11px] text-slate-400">（录音或打字）</span>
            </div>

            <textarea
              value={textAnswer}
              onChange={(e) => setTextAnswer(e.target.value)}
              placeholder="在此输入你的回答..."
              rows={4}
              className="w-full rounded-xl border border-slate-200 bg-surface-muted px-4 py-3 text-sm text-slate-700 placeholder-slate-400 outline-none focus:border-accent/50 resize-none mb-3"
            />

            <div className="flex items-center gap-3">
              <button
                onClick={isRecording ? stopRecording : startRecording}
                disabled={isSubmitting}
                className={`flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-all ${
                  isRecording
                    ? 'bg-red-50 text-red-600 border border-red-200'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                }`}
              >
                {isRecording ? <Square size={14} /> : <Mic size={14} />}
                {isRecording ? '停止录音' : '录音作答'}
              </button>

              <button
                onClick={() => submitAnswer(textAnswer)}
                disabled={!textAnswer.trim() || isSubmitting}
                className="flex items-center gap-2 rounded-xl bg-accent px-5 py-2.5 text-sm font-medium text-white hover:bg-accent-dark disabled:opacity-40 transition-all"
              >
                {isSubmitting ? (
                  <div className="h-3.5 w-3.5 rounded-full border-2 border-white/30 border-t-white animate-spin" />
                ) : (
                  <ArrowRight size={14} />
                )}
                提交回答
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Feedback phase
  if (phase === 'feedback' && currentFeedback) {
    return (
      <div className="flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto max-w-2xl space-y-4 animate-slide-up">
          {/* Progress */}
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-400">已完成 {progress.current} / {progress.total} 题</span>
            <div className="h-1.5 flex-1 mx-4 rounded-full bg-slate-100 overflow-hidden">
              <div
                className="h-full rounded-full bg-gradient-to-r from-accent to-cyan transition-all"
                style={{ width: `${(progress.current / progress.total) * 100}%` }}
              />
            </div>
          </div>

          {/* Question & Answer */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
            <p className="text-sm font-medium text-primary mb-2">{currentFeedback.question}</p>
            <div className="rounded-lg bg-surface-muted p-3">
              <p className="text-xs text-slate-600 whitespace-pre-wrap">{currentFeedback.answer}</p>
            </div>
          </div>

          {/* Defects */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
            <div className="flex items-center gap-2 mb-3">
              {currentFeedback.defects.length === 0 ? (
                <CheckCircle size={14} className="text-emerald-500" />
              ) : (
                <AlertTriangle size={14} className="text-amber-500" />
              )}
              <span className="text-sm font-medium text-primary">{currentFeedback.summary}</span>
            </div>

            {currentFeedback.defects.length > 0 && (
              <div className="space-y-2.5">
                {currentFeedback.defects.map((d) => (
                  <div key={d.id} className={`rounded-xl p-3 border ${
                    d.severity === 'critical' ? 'border-red-200 bg-red-50/50' :
                    d.severity === 'moderate' ? 'border-amber-200 bg-amber-50/50' :
                    'border-slate-200 bg-slate-50/50'
                  }`}>
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`h-1.5 w-1.5 rounded-full ${
                        d.severity === 'critical' ? 'bg-red-500' :
                        d.severity === 'moderate' ? 'bg-amber-500' : 'bg-slate-400'
                      }`} />
                      <span className="text-xs font-medium text-slate-700">{d.description}</span>
                    </div>
                    <p className="text-[11px] text-slate-500 ml-3.5">{d.suggestion}</p>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex justify-center gap-3">
            {hasNext ? (
              <button
                onClick={nextQuestion}
                className="flex items-center gap-2 rounded-xl bg-accent px-6 py-2.5 text-sm font-medium text-white hover:bg-accent-dark transition-all"
              >
                <ArrowRight size={14} />
                下一题
              </button>
            ) : (
              <button
                onClick={showReport}
                className="flex items-center gap-2 rounded-xl bg-gradient-to-r from-accent to-cyan px-6 py-2.5 text-sm font-medium text-white shadow-elevated hover:shadow-glow transition-all"
              >
                <BarChart3 size={14} />
                查看总结报告
              </button>
            )}
          </div>
        </div>
      </div>
    );
  }

  // Report phase
  if (phase === 'report' && report) {
    return (
      <div className="flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto max-w-2xl space-y-4 animate-slide-up">
          {/* Score card */}
          <div className="rounded-2xl border border-accent/20 bg-gradient-to-r from-white to-accent/5 p-6 shadow-card text-center">
            <h3 className="text-sm font-semibold text-slate-500 mb-2">面试总评</h3>
            <div className="text-5xl font-bold text-accent mb-1">{report.overallScore}</div>
            <div className="text-xs text-slate-400">/ 10 分</div>
            <p className="mt-3 text-sm text-slate-600">
              共回答 {report.totalQuestions} 题，检出 {report.totalDefects} 个缺陷
            </p>
          </div>

          {/* Severity breakdown */}
          <div className="grid grid-cols-3 gap-3">
            <div className="rounded-xl border border-red-200 bg-white p-3 text-center shadow-card">
              <XCircle size={16} className="text-red-500 mx-auto mb-1" />
              <div className="text-lg font-bold text-red-600">{report.bySeverity.critical}</div>
              <div className="text-[11px] text-slate-500">严重</div>
            </div>
            <div className="rounded-xl border border-amber-200 bg-white p-3 text-center shadow-card">
              <AlertTriangle size={16} className="text-amber-500 mx-auto mb-1" />
              <div className="text-lg font-bold text-amber-600">{report.bySeverity.moderate}</div>
              <div className="text-[11px] text-slate-500">中等</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-3 text-center shadow-card">
              <CheckCircle size={16} className="text-slate-400 mx-auto mb-1" />
              <div className="text-lg font-bold text-slate-600">{report.bySeverity.minor}</div>
              <div className="text-[11px] text-slate-500">轻微</div>
            </div>
          </div>

          {/* Top issues */}
          {report.topIssues.length > 0 && (
            <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
              <h4 className="text-sm font-medium text-primary mb-3">高频问题</h4>
              <div className="space-y-2">
                {report.topIssues.map((issue, i) => (
                  <div key={i} className="flex items-center justify-between rounded-lg bg-surface-muted px-3 py-2">
                    <span className="text-xs text-slate-600">{issue.type}</span>
                    <span className="text-xs font-semibold text-accent">{issue.count} 次</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* History */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-card">
            <h4 className="text-sm font-medium text-primary mb-3">逐题回顾</h4>
            <div className="space-y-3">
              {history.map((h, i) => (
                <div key={i} className="rounded-xl border border-slate-100 p-3">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-[11px] font-bold text-accent">Q{i + 1}</span>
                    <span className="text-xs text-slate-600">{h.question}</span>
                  </div>
                  <div className="flex items-center gap-2 mt-1">
                    {h.defects.length === 0 ? (
                      <span className="text-[11px] text-emerald-500">无缺陷</span>
                    ) : (
                      <span className="text-[11px] text-amber-500">{h.defects.length} 个缺陷</span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Restart */}
          <div className="flex justify-center">
            <button
              onClick={resetInterview}
              className="flex items-center gap-2 rounded-xl border border-slate-200 px-6 py-2.5 text-sm text-slate-600 hover:bg-slate-50 transition-all"
            >
              再来一次
            </button>
          </div>
        </div>
      </div>
    );
  }

  return null;
}

function encodeWav(chunks: Float32Array[], sampleRate: number): Blob {
  const length = chunks.reduce((total, chunk) => total + chunk.length, 0);
  const pcm = new Int16Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    for (let i = 0; i < chunk.length; i++) {
      const sample = Math.max(-1, Math.min(1, chunk[i]));
      pcm[offset++] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
    }
  }
  const buffer = new ArrayBuffer(44 + pcm.length * 2);
  const view = new DataView(buffer);
  const writeStr = (off: number, s: string) => { for (let i = 0; i < s.length; i++) view.setUint8(off + i, s.charCodeAt(i)); };
  writeStr(0, 'RIFF');
  view.setUint32(4, 36 + pcm.length * 2, true);
  writeStr(8, 'WAVE');
  writeStr(12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeStr(36, 'data');
  view.setUint32(40, pcm.length * 2, true);
  let byteOffset = 44;
  for (let i = 0; i < pcm.length; i++) { view.setInt16(byteOffset, pcm[i], true); byteOffset += 2; }
  return new Blob([buffer], { type: 'audio/wav' });
}
