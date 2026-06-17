'use client';

import { RotateCcw, Brain } from 'lucide-react';

interface Props {
  onReset: () => void;
  model: string;
  onModelChange: (model: string) => void;
}

const MODELS = [
  { id: 'gpt-5.5', label: 'GPT-5.5' },
  { id: 'claude', label: 'Claude' },
  { id: 'openai', label: 'OpenAI Default' },
  { id: 'deepseek', label: 'DeepSeek' },
];

export function Sidebar({ onReset, model, onModelChange }: Props) {
  return (
    <aside className="hidden w-64 flex-col border-r border-zinc-800 bg-surface p-4 md:flex">
      <div className="mb-8 flex items-center gap-2">
        <Brain className="text-primary" size={24} />
        <span className="text-lg font-semibold">面试诊断</span>
      </div>

      <div className="mb-6">
        <label className="mb-2 block text-xs font-medium text-zinc-400">模型选择</label>
        <div className="space-y-1">
          {MODELS.map((m) => (
            <button
              key={m.id}
              onClick={() => onModelChange(m.id)}
              className={`w-full rounded-lg px-3 py-2 text-left text-sm transition ${
                model === m.id
                  ? 'bg-primary/20 text-primary'
                  : 'text-zinc-400 hover:bg-surface-light hover:text-zinc-200'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>
      </div>

      <div className="mb-6">
        <label className="mb-2 block text-xs font-medium text-zinc-400">考察维度</label>
        <div className="space-y-1 text-xs text-zinc-500">
          <p>架构设计 / Harness 工程</p>
          <p>模型能力 / RAG</p>
          <p>多 Agent / 评测 / 全栈</p>
        </div>
      </div>

      <div className="mt-auto">
        <button
          onClick={onReset}
          className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-zinc-400 transition hover:bg-surface-light hover:text-zinc-200"
        >
          <RotateCcw size={14} />
          新建会话
        </button>
      </div>
    </aside>
  );
}
