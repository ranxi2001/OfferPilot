'use client';

import { MessageSquare, FileText, GitCompare, BarChart3, Compass, Settings, Mic } from 'lucide-react';

export type ViewType = 'chat' | 'interview' | 'resume' | 'match' | 'dashboard';

interface Props {
  activeView: ViewType;
  onViewChange: (view: ViewType) => void;
  model: string;
  onModelChange: (model: string) => void;
}

const NAV_ITEMS: { id: ViewType; label: string; icon: typeof MessageSquare; desc: string }[] = [
  { id: 'chat', label: '面试诊断', icon: MessageSquare, desc: '对话式诊断' },
  { id: 'interview', label: '模拟面试', icon: Mic, desc: '逐题实时反馈' },
  { id: 'resume', label: '简历分析', icon: FileText, desc: '段落级优化' },
  { id: 'match', label: 'JD 匹配', icon: GitCompare, desc: '关键词覆盖' },
  { id: 'dashboard', label: '能力雷达', icon: BarChart3, desc: '7 维度评估' },
];

const MODELS = [
  { id: 'claude', label: 'Claude' },
  { id: 'gpt-4o', label: 'GPT-4o' },
  { id: 'deepseek', label: 'DeepSeek' },
];

export function Sidebar({ activeView, onViewChange, model, onModelChange }: Props) {
  return (
    <aside className="hidden w-72 flex-col border-r border-slate-200/80 bg-white/70 backdrop-blur-sm md:flex">
      {/* Logo */}
      <div className="flex items-center gap-3 px-6 py-5 border-b border-slate-100">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-navy-900 to-accent shadow-card">
          <Compass size={18} className="text-white" />
        </div>
        <div>
          <h1 className="text-base font-bold text-primary">OfferPilot</h1>
          <p className="text-[11px] text-slate-400">AI Interview Diagnosis</p>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 px-3 py-4 space-y-1">
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon;
          const isActive = activeView === item.id;
          return (
            <button
              key={item.id}
              onClick={() => onViewChange(item.id)}
              className={`group flex w-full items-center gap-3 rounded-xl px-4 py-3 text-left transition-all ${
                isActive
                  ? 'bg-accent/10 text-accent shadow-sm border border-accent/20'
                  : 'text-slate-600 hover:bg-slate-50 hover:text-primary border border-transparent'
              }`}
            >
              <div className={`flex h-8 w-8 items-center justify-center rounded-lg ${
                isActive ? 'bg-accent/15' : 'bg-slate-100 group-hover:bg-slate-200'
              }`}>
                <Icon size={16} className={isActive ? 'text-accent' : 'text-slate-500 group-hover:text-primary'} />
              </div>
              <div>
                <div className={`text-sm font-medium ${isActive ? 'text-accent-dark' : ''}`}>{item.label}</div>
                <div className="text-[11px] text-slate-400">{item.desc}</div>
              </div>
            </button>
          );
        })}
      </nav>

      {/* Model selector */}
      <div className="px-4 py-4 border-t border-slate-100">
        <div className="flex items-center gap-2 mb-2 px-2">
          <Settings size={12} className="text-slate-400" />
          <span className="text-xs font-medium text-slate-400">模型</span>
        </div>
        <div className="flex gap-1 rounded-lg bg-slate-100 p-1">
          {MODELS.map((m) => (
            <button
              key={m.id}
              onClick={() => onModelChange(m.id)}
              className={`flex-1 rounded-md px-2 py-1.5 text-xs font-medium transition-all ${
                model === m.id
                  ? 'bg-white text-primary shadow-sm'
                  : 'text-slate-500 hover:text-slate-700'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>
      </div>
    </aside>
  );
}
