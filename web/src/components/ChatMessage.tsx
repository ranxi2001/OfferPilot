'use client';

import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Bot, Check, Clipboard, Download, FileText, Mic, User, Wrench } from 'lucide-react';
import { ThinkingBlock } from './ThinkingBlock';

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'process';
  content: string;
  thinking?: string;
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
  isThinking?: boolean;
}

export function ChatMessage({ message, isStreaming, isThinking }: Props) {
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
    a.download = `offerpilot-${new Date().toISOString().slice(0, 10)}.md`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  if (isUser) {
    return (
      <div className="flex justify-end animate-slide-up">
        <div className="flex items-start gap-3 max-w-[75%]">
          <div className="rounded-2xl rounded-tr-md bg-primary px-4 py-3 text-sm text-white shadow-card">
            <p className="whitespace-pre-wrap leading-relaxed">{message.content}</p>
          </div>
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10">
            <User size={14} className="text-primary" />
          </div>
        </div>
      </div>
    );
  }

  if (isProcess) {
    return (
      <div className="flex animate-slide-up">
        <div className="flex items-start gap-3 max-w-[80%]">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-cyan/10">
            <Mic size={14} className="text-cyan" />
          </div>
          <div className={`rounded-2xl rounded-tl-md px-4 py-3 shadow-card border ${
            message.isError
              ? 'border-red-200 bg-red-50 text-red-700'
              : 'border-cyan/20 bg-cyan-light/5 text-slate-700'
          }`}>
            <div className="flex items-center gap-2 mb-2">
              <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                message.isError ? 'bg-red-100 text-red-600' : 'bg-cyan/10 text-cyan-dark'
              }`}>
                {message.status ?? '处理中'}
              </span>
              {message.audioUrl && (
                <a
                  href={message.audioUrl}
                  download={message.audioName ?? 'recording.wav'}
                  className="inline-flex items-center gap-1 text-[11px] text-slate-500 hover:text-accent"
                >
                  <Download size={10} />
                  保存录音
                </a>
              )}
            </div>
            {message.audioUrl && <audio src={message.audioUrl} controls className="h-8 w-full mb-2" />}
            {message.transcript && (
              <div className="rounded-lg border border-slate-200 bg-white/80 p-2.5 mb-2">
                <div className="text-[11px] text-slate-400 mb-1">转写文本</div>
                <p className="text-xs text-slate-600 whitespace-pre-wrap">{message.transcript}</p>
              </div>
            )}
            <p className="text-sm text-slate-600">{message.content}</p>
          </div>
        </div>
      </div>
    );
  }

  // Assistant message
  return (
    <div className="flex animate-slide-up">
      <div className="flex items-start gap-3 max-w-[85%]">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-accent to-cyan shadow-sm">
          <Bot size={14} className="text-white" />
        </div>
        <div className="flex-1 min-w-0">
          {/* Thinking block */}
          {(message.thinking || isThinking) && (
            <ThinkingBlock content={message.thinking ?? ''} isStreaming={isThinking} />
          )}

          {/* Tool calls */}
          {message.toolCalls && message.toolCalls.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-1.5">
              {message.toolCalls.map((tool, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1 rounded-full bg-accent/8 border border-accent/15 px-2.5 py-0.5 text-[11px] font-medium text-accent-dark"
                >
                  <Wrench size={10} />
                  {tool}
                </span>
              ))}
            </div>
          )}

          {/* Content */}
          <div className={`rounded-2xl rounded-tl-md px-4 py-3 shadow-card border border-slate-100 ${
            message.isError ? 'bg-red-50 border-red-200' : 'bg-white'
          }`}>
            <div className={`markdown-body text-sm leading-relaxed ${
              isStreaming && !message.content ? 'cursor-blink' : ''
            } ${message.isError ? 'text-red-700' : 'text-slate-700'}`}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
            </div>
          </div>

          {/* Actions */}
          {message.content && !isStreaming && (
            <div className="mt-2 flex gap-1.5">
              <button
                type="button"
                onClick={copyContent}
                className="inline-flex items-center gap-1 rounded-lg px-2.5 py-1 text-[11px] text-slate-400 hover:bg-slate-100 hover:text-slate-600 transition-all"
              >
                {copied ? <Check size={11} className="text-green-500" /> : <Clipboard size={11} />}
                {copied ? '已复制' : '复制'}
              </button>
              <button
                type="button"
                onClick={downloadMarkdown}
                className="inline-flex items-center gap-1 rounded-lg px-2.5 py-1 text-[11px] text-slate-400 hover:bg-slate-100 hover:text-slate-600 transition-all"
              >
                <FileText size={11} />
                导出
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
