'use client';

import { useState } from 'react';
import { Download, FileText, Loader2 } from 'lucide-react';

interface Props {
  messages: { role: string; content: string; thinking?: string; toolCalls?: string[] }[];
  sessionId: string | null;
}

export function ExportButton({ messages, sessionId }: Props) {
  const [isExporting, setIsExporting] = useState(false);
  const [showMenu, setShowMenu] = useState(false);

  const assistantMessages = messages.filter((m) => m.role === 'assistant' && m.content);

  if (assistantMessages.length === 0) return null;

  function exportMarkdown() {
    const md = buildMarkdown(messages, sessionId);
    const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' });
    download(blob, `offerpilot-report-${dateStr()}.md`);
    setShowMenu(false);
  }

  async function exportPDF() {
    setIsExporting(true);
    setShowMenu(false);

    try {
      const md = buildMarkdown(messages, sessionId);
      const html = buildPDFHtml(md);

      const printWindow = window.open('', '_blank');
      if (!printWindow) {
        alert('请允许弹出窗口以导出 PDF');
        return;
      }

      printWindow.document.write(html);
      printWindow.document.close();

      printWindow.onload = () => {
        setTimeout(() => {
          printWindow.print();
          printWindow.close();
        }, 500);
      };
    } finally {
      setIsExporting(false);
    }
  }

  return (
    <div className="relative">
      <button
        onClick={() => setShowMenu(!showMenu)}
        disabled={isExporting}
        className="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs text-slate-500 hover:bg-slate-100 hover:text-primary transition-all"
      >
        {isExporting ? (
          <Loader2 size={12} className="animate-spin" />
        ) : (
          <Download size={12} />
        )}
        导出报告
      </button>

      {showMenu && (
        <div className="absolute right-0 top-full mt-1 z-10 rounded-xl border border-slate-200 bg-white shadow-elevated py-1 min-w-[140px] animate-fade-in">
          <button
            onClick={exportMarkdown}
            className="flex w-full items-center gap-2 px-3 py-2 text-xs text-slate-600 hover:bg-slate-50"
          >
            <FileText size={12} />
            Markdown (.md)
          </button>
          <button
            onClick={exportPDF}
            className="flex w-full items-center gap-2 px-3 py-2 text-xs text-slate-600 hover:bg-slate-50"
          >
            <Download size={12} />
            PDF 打印
          </button>
        </div>
      )}
    </div>
  );
}

function dateStr() {
  return new Date().toISOString().slice(0, 10);
}

function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

function buildMarkdown(messages: { role: string; content: string; thinking?: string; toolCalls?: string[] }[], sessionId: string | null): string {
  const lines: string[] = [];
  lines.push('# OfferPilot 面试诊断报告\n');
  lines.push(`**日期**: ${new Date().toLocaleDateString('zh-CN')}`);
  if (sessionId) lines.push(`**会话**: ${sessionId.slice(0, 8)}`);
  lines.push(`**诊断轮次**: ${messages.filter((m) => m.role === 'user').length}`);
  lines.push('\n---\n');

  let questionIndex = 0;
  for (const msg of messages) {
    if (msg.role === 'user') {
      questionIndex++;
      lines.push(`## 问题 ${questionIndex}\n`);
      lines.push(`> ${msg.content}\n`);
    } else if (msg.role === 'assistant' && msg.content) {
      lines.push(msg.content);
      lines.push('\n---\n');
    }
  }

  lines.push('\n*由 OfferPilot AI Interview Diagnosis Agent 生成*');
  return lines.join('\n');
}

function buildPDFHtml(markdown: string): string {
  const escaped = markdown
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  const html = escaped
    .replace(/^# (.+)$/gm, '<h1>$1</h1>')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/^&gt; (.+)$/gm, '<blockquote>$1</blockquote>')
    .replace(/^- (.+)$/gm, '<li>$1</li>')
    .replace(/^---$/gm, '<hr/>')
    .replace(/\n{2,}/g, '</p><p>')
    .replace(/\n/g, '<br/>');

  return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>OfferPilot 诊断报告</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 800px; margin: 0 auto; padding: 40px; color: #1e293b; line-height: 1.6; }
  h1 { color: #1e3a5f; border-bottom: 2px solid #0ea5e9; padding-bottom: 8px; }
  h2 { color: #1e3a5f; margin-top: 24px; }
  h3 { color: #475569; }
  blockquote { border-left: 3px solid #0ea5e9; padding-left: 12px; color: #475569; background: #f0f7ff; padding: 8px 12px; border-radius: 4px; }
  hr { border: none; border-top: 1px solid #e2e8f0; margin: 20px 0; }
  strong { color: #1e3a5f; }
  code { background: #f1f5f9; padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
  table { border-collapse: collapse; width: 100%; margin: 12px 0; }
  th, td { border: 1px solid #e2e8f0; padding: 8px 12px; text-align: left; }
  th { background: #f0f7ff; font-weight: 600; }
  li { margin: 4px 0; }
  @media print { body { padding: 20px; } }
</style>
</head>
<body><p>${html}</p></body>
</html>`;
}
