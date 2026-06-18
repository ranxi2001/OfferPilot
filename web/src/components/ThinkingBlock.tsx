'use client';

import { useState } from 'react';
import { Brain, ChevronDown, ChevronRight } from 'lucide-react';

interface Props {
  content: string;
  isStreaming?: boolean;
}

export function ThinkingBlock({ content, isStreaming }: Props) {
  const [expanded, setExpanded] = useState(false);

  if (!content && !isStreaming) return null;

  return (
    <div className="animate-fade-in mb-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium text-slate-500 hover:bg-slate-100 hover:text-slate-700 transition-all"
      >
        <Brain size={13} className={isStreaming ? 'text-accent animate-pulse-slow' : 'text-slate-400'} />
        <span>{isStreaming ? '思考中' : '思考过程'}</span>
        {isStreaming && <span className="thinking-dots text-accent" />}
        {!isStreaming && (
          expanded
            ? <ChevronDown size={12} />
            : <ChevronRight size={12} />
        )}
        {!isStreaming && !expanded && (
          <span className="text-[11px] text-slate-400 ml-1">
            {content.length > 60 ? content.slice(0, 60) + '...' : content}
          </span>
        )}
      </button>

      {(expanded || isStreaming) && (
        <div className="mt-1 ml-3 rounded-lg border border-slate-200/80 bg-slate-50/80 px-4 py-3 animate-slide-up">
          <p className="text-xs leading-relaxed text-slate-500 whitespace-pre-wrap font-light italic">
            {content}
            {isStreaming && <span className="cursor-blink" />}
          </p>
        </div>
      )}
    </div>
  );
}
