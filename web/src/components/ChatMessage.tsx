'use client';

import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Bot, Check, Clipboard, Download, FileText, Mic, User, Wrench } from 'lucide-react';

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'process';
  content: string;
  toolCalls?: string[];
  isError?: boolean;
  audioUrl?: string;
  audioName?: string;
  transcript?: string;
  status?: string;
}

interface Props {
  message: Message;
  isStreaming?: boolean;
}

export function ChatMessage({ message, isStreaming }: Props) {
  const isUser = message.role === 'user';
  const isProcess = message.role === 'process';
  const [copied, setCopied] = useState(false);

  async function copyContent() {
    await navigator.clipboard.writeText(message.content);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  function downloadMarkdown() {
    const blob = new Blob([message.content], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `offerpilot-diagnosis-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.md`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  return (
    <div className={`flex gap-3 ${isUser ? 'flex-row-reverse' : ''}`}>
      <div
        className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full ${
          isUser ? 'bg-blue-600' : isProcess ? 'bg-emerald-600' : 'bg-primary'
        }`}
      >
        {isUser ? <User size={16} /> : isProcess ? <Mic size={16} /> : <Bot size={16} />}
      </div>

      <div
        className={`max-w-[80%] rounded-xl px-4 py-3 ${
          isUser
            ? 'bg-blue-600/20 text-zinc-100'
            : isProcess
              ? message.isError
                ? 'border border-red-500/30 bg-red-950/20 text-red-200'
                : 'border border-emerald-500/20 bg-emerald-950/20 text-zinc-200'
              : message.isError
                ? 'bg-red-900/20 text-red-300'
                : 'bg-surface-light text-zinc-200'
        }`}
      >
        {message.toolCalls && message.toolCalls.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-1">
            {message.toolCalls.map((tool, i) => (
              <span
                key={i}
                className="inline-flex items-center gap-1 rounded bg-primary/20 px-2 py-0.5 text-xs text-primary"
              >
                <Wrench size={10} />
                {tool}
              </span>
            ))}
          </div>
        )}

        {isProcess && (
          <div className="mb-3 space-y-3 text-sm">
            <div className="flex flex-wrap items-center gap-2">
              <span className="rounded bg-emerald-500/15 px-2 py-0.5 text-xs text-emerald-300">
                思维链 · {message.status ?? '处理中'}
              </span>
              {message.audioUrl && (
                <a
                  href={message.audioUrl}
                  download={message.audioName ?? 'recording.wav'}
                  className="inline-flex items-center gap-1 rounded bg-zinc-800 px-2 py-1 text-xs text-zinc-300 transition hover:text-white"
                >
                  <Download size={12} />
                  保存录音
                </a>
              )}
            </div>
            {message.audioUrl && <audio src={message.audioUrl} controls className="h-9 w-full" />}
            {message.transcript && (
              <div className="rounded-lg border border-zinc-700/70 bg-zinc-950/40 p-3">
                <div className="mb-1 text-xs text-zinc-500">转写文本</div>
                <p className="whitespace-pre-wrap text-zinc-200">{message.transcript}</p>
              </div>
            )}
          </div>
        )}

        <div className={`markdown-body text-sm leading-relaxed ${isStreaming && !message.content ? 'cursor-blink' : ''}`}>
          {isUser ? (
            <p className="whitespace-pre-wrap">{message.content}</p>
          ) : isProcess ? (
            <p className="whitespace-pre-wrap text-zinc-300">{message.content}</p>
          ) : (
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
          )}
        </div>

        {!isUser && !isProcess && message.content && !isStreaming && (
          <div className="mt-3 flex flex-wrap gap-2 border-t border-zinc-700/70 pt-3">
            <button
              type="button"
              onClick={copyContent}
              className="inline-flex items-center gap-1 rounded bg-zinc-800 px-2 py-1 text-xs text-zinc-300 transition hover:text-white"
            >
              {copied ? <Check size={12} /> : <Clipboard size={12} />}
              {copied ? '已复制' : '复制'}
            </button>
            <button
              type="button"
              onClick={downloadMarkdown}
              className="inline-flex items-center gap-1 rounded bg-zinc-800 px-2 py-1 text-xs text-zinc-300 transition hover:text-white"
            >
              <FileText size={12} />
              保存 .md
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
