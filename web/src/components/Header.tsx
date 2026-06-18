'use client';

import { MessageSquare, FileText, GitCompare, BarChart3, Mic, RotateCcw, Menu } from 'lucide-react';
import type { ViewType } from './Sidebar';

interface Props {
  activeView: ViewType;
  sessionId: string | null;
  questionsCount: number;
  onReset: () => void;
  onMobileMenu?: () => void;
}

const VIEW_TITLES: Record<ViewType, { title: string; icon: typeof MessageSquare }> = {
  chat: { title: '面试诊断', icon: MessageSquare },
  interview: { title: '模拟面试', icon: Mic },
  resume: { title: '简历分析', icon: FileText },
  match: { title: 'JD 匹配', icon: GitCompare },
  dashboard: { title: '能力雷达', icon: BarChart3 },
};

export function Header({ activeView, sessionId, questionsCount, onReset, onMobileMenu }: Props) {
  const { title, icon: Icon } = VIEW_TITLES[activeView];

  return (
    <header className="flex items-center justify-between border-b border-slate-200/80 bg-white/60 backdrop-blur-sm px-5 py-3">
      <div className="flex items-center gap-3">
        <button
          onClick={onMobileMenu}
          className="flex h-8 w-8 items-center justify-center rounded-lg hover:bg-slate-100 md:hidden"
        >
          <Menu size={18} className="text-slate-600" />
        </button>
        <div className="flex items-center gap-2">
          <Icon size={15} className="text-accent" />
          <span className="text-sm font-semibold text-primary">{title}</span>
        </div>
        {activeView === 'chat' && sessionId && (
          <span className="hidden sm:inline rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] text-slate-400">
            {sessionId.slice(0, 8)}
          </span>
        )}
      </div>

      <div className="flex items-center gap-3">
        {activeView === 'chat' && questionsCount > 0 && (
          <span className="text-xs text-slate-400">
            {questionsCount} 轮对话
          </span>
        )}
        {activeView === 'chat' && (
          <button
            onClick={onReset}
            className="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs text-slate-500 hover:bg-slate-100 hover:text-primary transition-all"
          >
            <RotateCcw size={12} />
            新会话
          </button>
        )}
      </div>
    </header>
  );
}
