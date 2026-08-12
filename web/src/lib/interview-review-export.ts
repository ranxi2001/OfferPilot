import type { InterviewExecutionRun, InterviewReviewSnapshot } from '@/types/interview';

export interface ReviewRecording {
  questionId: string;
  dataUrl: string;
  mimeType: string;
  durationMs: number;
}

export interface InterviewReviewExportInput {
  review: InterviewReviewSnapshot;
  recordings: ReviewRecording[];
  executionRuns: InterviewExecutionRun[];
}

export async function recordingToDataUrl(questionId: string, blob: Blob, durationMs: number): Promise<ReviewRecording> {
  return {
    questionId,
    dataUrl: await blobToDataUrl(blob),
    mimeType: blob.type || 'audio/wav',
    durationMs,
  };
}

export function buildInterviewReviewHtml(input: InterviewReviewExportInput): string {
  const recordings = new Map(input.recordings.map((recording) => [recording.questionId, recording]));
  const traceByTurn = new Map<number, InterviewExecutionRun[]>();
  for (const run of input.executionRuns) {
    const match = run.action === 'answer' ? run.title.match(/第\s*(\d+)\s*轮/) : null;
    const index = match ? Number(match[1]) : 0;
    if (!index) continue;
    traceByTurn.set(index, [...(traceByTurn.get(index) ?? []), run]);
  }

  const turns = input.review.turns.map((turn, turnIndex) => {
    const index = turnIndex + 1;
    const recording = recordings.get(turn.question.id);
    const references = turn.references.length > 0
      ? turn.references.map((reference) => `<article class="reference"><h4>${escapeHtml(reference.title || '参考答案')}</h4><p>${multiline(reference.answer)}</p>${reference.locator ? `<small>${escapeHtml(reference.locator)}</small>` : ''}</article>`).join('')
      : '<p class="empty">本题没有绑定知识库标准答案，请结合材料核对与回答分析复盘。</p>';
    const traces = (traceByTurn.get(index) ?? []).flatMap((run) => run.steps.map((step) => `
      <li><span>${escapeHtml(step.label)}</span><small>${escapeHtml(step.detail || step.status)}${step.durationMs ? ` · ${step.durationMs} ms` : ''}</small></li>`)).join('');
    return `<section class="turn">
      <header><span>第 ${index} 题</span><strong>${turn.feedback.score} / 100</strong></header>
      <h2>${escapeHtml(turn.question.text)}</h2>
      <div class="meta">${escapeHtml(turn.question.topic)} · ${escapeHtml(turn.inputMode === 'voice' ? '语音回答' : '文本回答')} · ${formatDuration(turn.durationMs)}</div>
      <div class="grid"><article><h3>原始回答 / 转写</h3><p>${multiline(turn.answer)}</p></article>
      <article><h3>回答录音</h3>${recording ? `<audio controls preload="metadata" src="${escapeAttribute(recording.dataUrl)}"></audio>` : '<p class="empty">本轮没有可导出的录音，或页面刷新后内存录音已释放。</p>'}</article></div>
      <article><h3>回答分析</h3><p>${multiline(turn.feedback.summary)}</p>${list('站得住的部分', turn.feedback.strengths)}${list('继续追的漏洞', turn.feedback.gaps)}${turn.feedback.coachTip ? `<div class="coach"><b>下一次这样答</b><p>${multiline(turn.feedback.coachTip)}</p></div>` : ''}</article>
      <article><h3>标准答案与知识证据</h3>${references}</article>
      <details><summary>Agent 安全执行轨迹</summary>${traces ? `<ol class="trace">${traces}</ol>` : '<p class="empty">当前页面没有本轮执行轨迹。</p>'}</details>
    </section>`;
  }).join('');

  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>OfferPilot 模拟面试复盘</title><style>${reviewStyles}</style></head><body><main><header class="cover"><div><span class="brand">OfferPilot · REVIEW</span><h1>模拟面试复盘档案</h1><p>截至导出时的题目、回答、录音、分析、标准答案与安全执行轨迹。</p></div><dl><div><dt>面试编号</dt><dd>${escapeHtml(input.review.interviewId)}</dd></div><div><dt>已完成</dt><dd>${input.review.turns.length} 轮</dd></div><div><dt>导出时间</dt><dd>${escapeHtml(formatDate(input.review.generatedAt))}</dd></div><div><dt>Schema</dt><dd>${escapeHtml(input.review.schemaVersion)}</dd></div></dl></header>${turns || '<section class="turn"><p class="empty">尚无已提交回答。</p></section>'}<footer>该文件包含面试回答、录音和私有学习参考，请妥善保存。Agent 轨迹仅含安全执行事件，不含模型私有思维链。</footer></main></body></html>`;
}

export function downloadInterviewReview(html: string, interviewId: string): void {
  const url = URL.createObjectURL(new Blob([html], { type: 'text/html;charset=utf-8' }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `offerpilot-review-${safeFilename(interviewId)}-${new Date().toISOString().slice(0, 10)}.html`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error ?? new Error('Could not read recording'));
    reader.readAsDataURL(blob);
  });
}

function list(title: string, values: string[]): string {
  return values.length ? `<h4>${title}</h4><ul>${values.map((value) => `<li>${escapeHtml(value)}</li>`).join('')}</ul>` : '';
}
function multiline(value: string): string { return escapeHtml(value).replace(/\n/g, '<br>'); }
function escapeHtml(value: string): string { return value.replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]!); }
function escapeAttribute(value: string): string { return escapeHtml(value); }
function safeFilename(value: string): string { return value.replace(/[^A-Za-z0-9._-]/g, '-').slice(0, 80) || 'interview'; }
function formatDuration(value?: number): string { return value ? `${Math.round(value / 1000)} 秒` : '未记录时长'; }
function formatDate(value: string): string { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN'); }

const reviewStyles = `:root{color-scheme:light;--ink:#172033;--muted:#64748b;--line:#dbe2ea;--paper:#fff;--canvas:#eef2f5;--accent:#087f5b;--amber:#b45309}*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);font-family:"Noto Sans SC","Microsoft YaHei",sans-serif;line-height:1.7}main{width:min(980px,100%);margin:auto;background:var(--paper);min-height:100vh}.cover{padding:54px 56px 36px;border-top:8px solid var(--accent);display:grid;grid-template-columns:1.4fr 1fr;gap:36px;border-bottom:1px solid var(--line)}.brand{font:700 12px ui-monospace,monospace;color:var(--accent);letter-spacing:.12em}.cover h1{font-size:38px;line-height:1.2;margin:14px 0 10px}.cover p,.meta,.empty,small{color:var(--muted)}dl{margin:0;display:grid;gap:10px}dl div{border-bottom:1px solid var(--line);padding-bottom:8px}dt{font-size:11px;color:var(--muted)}dd{margin:2px 0 0;font-weight:700;font-size:13px;overflow-wrap:anywhere}.turn{margin:0 56px;padding:42px 0;border-bottom:2px solid var(--ink)}.turn>header{display:flex;justify-content:space-between;color:var(--accent);font-weight:700}.turn h2{font-size:24px;line-height:1.45;margin:10px 0}.turn h3{font-size:15px;margin:0 0 10px;border-left:3px solid var(--accent);padding-left:10px}.turn h4{font-size:13px;margin:16px 0 4px}.grid{display:grid;grid-template-columns:1.5fr 1fr;gap:16px}article,details{margin-top:18px;padding:18px;border:1px solid var(--line);background:#fbfcfd;border-radius:6px}article p{margin:0;overflow-wrap:anywhere}audio{width:100%}.reference{margin-top:10px;background:#f0fdf7;border-color:#b7e4d2}.coach{margin-top:16px;padding:12px 14px;background:#fff7ed;border-left:3px solid var(--amber)}ul{padding-left:22px}.trace{padding-left:22px}.trace li{margin:9px 0}.trace span,.trace small{display:block}.trace span{font-weight:700;font-size:13px}summary{cursor:pointer;font-weight:700}footer{padding:32px 56px 48px;color:var(--muted);font-size:12px}@media(max-width:700px){.cover{padding:32px 22px;grid-template-columns:1fr}.turn{margin:0 22px}.grid{grid-template-columns:1fr}footer{padding:28px 22px}}@media print{body{background:white}.turn{break-inside:avoid}details{display:block}}`;
