'use client';

import { useEffect, useState } from 'react';
import { MessageSquare, FileText, GitCompare, BarChart3, Settings, Mic, ChevronDown, Circle, Pencil } from 'lucide-react';

export type ViewType = 'chat' | 'interview' | 'resume' | 'match' | 'dashboard';

interface Props {
  activeView: ViewType;
  onViewChange: (view: ViewType) => void;
  model: string;
  onModelChange: (model: string) => void;
  onOpenConfig?: () => void;
}

const NAV_ITEMS: { id: ViewType; label: string; icon: typeof MessageSquare; desc: string }[] = [
  { id: 'chat', label: '面试诊断', icon: MessageSquare, desc: '对话式诊断' },
  { id: 'resume', label: '简历诊断', icon: FileText, desc: '上传 PDF 一键分析' },
  { id: 'match', label: 'JD 匹配', icon: GitCompare, desc: '简历-岗位对齐' },
  { id: 'interview', label: '模拟面试', icon: Mic, desc: '逐题实时反馈' },
  { id: 'dashboard', label: '能力雷达', icon: BarChart3, desc: '7 维度评估' },
];

interface ModelItem {
  name: string;
  provider: string;
  model: string;
  available: boolean;
}

interface ModelsConfig {
  text: ModelItem[];
  tts: ModelItem[];
  multimodal: ModelItem[];
}

export function Sidebar({ activeView, onViewChange, model, onModelChange, onOpenConfig }: Props) {
  const [config, setConfig] = useState<ModelsConfig | null>(null);
  const [expandedGroup, setExpandedGroup] = useState<string | null>(null);
  const [ttsModel, setTtsModel] = useState<string>('');
  const [mmModel, setMmModel] = useState<string>('');

  const loadConfig = () => {
    fetch('/api/config').then((r) => r.json()).then((data: ModelsConfig) => {
      setConfig(data);
      const firstText = data.text.find((m) => m.available);
      if (firstText && !model) onModelChange(firstText.model || firstText.name);
      const firstTts = data.tts.find((m) => m.available);
      if (firstTts) setTtsModel(firstTts.model || firstTts.name);
      const firstMm = data.multimodal.find((m) => m.available);
      if (firstMm) setMmModel(firstMm.model || firstMm.name);
    }).catch(() => {});
  };

  useEffect(() => { loadConfig(); }, []);

  const toggleGroup = (group: string) => {
    setExpandedGroup(expandedGroup === group ? null : group);
  };

  const groups = [
    { key: 'text', label: '文本模型', items: config?.text ?? [], selected: model, onSelect: onModelChange },
    { key: 'tts', label: 'TTS 语音', items: config?.tts ?? [], selected: ttsModel, onSelect: setTtsModel },
    { key: 'multimodal', label: '语音识别', items: config?.multimodal ?? [], selected: mmModel, onSelect: setMmModel },
  ];

  return (
    <aside className="hidden w-72 flex-col border-r border-slate-200/80 bg-white/70 backdrop-blur-sm md:flex">
      {/* Logo */}
      <div className="flex items-center gap-3 px-6 py-5 border-b border-slate-100">
        <div className="flex h-10 w-10 items-center justify-center overflow-hidden rounded-xl bg-white shadow-card ring-1 ring-slate-100">
          <img
            src="/brand/offerpilot-icon-192.png"
            alt="OfferPilot"
            className="h-full w-full object-cover"
          />
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

      {/* Model config panel */}
      <div className="px-3 py-3 border-t border-slate-100 space-y-1">
        <div className="flex items-center gap-2 px-3 mb-1">
          <Settings size={12} className="text-slate-400" />
          <span className="text-xs font-medium text-slate-400">模型配置</span>
          <button
            onClick={onOpenConfig}
            className="ml-auto flex items-center gap-1 text-[10px] text-slate-400 hover:text-accent transition-colors"
          >
            <Pencil size={10} />
            编辑
          </button>
        </div>

        {groups.map((group) => {
          const isExpanded = expandedGroup === group.key;
          const currentItem = group.items.find((m) => (m.model || m.name) === group.selected || m.name === group.selected);
          const availableCount = group.items.filter((m) => m.available).length;

          return (
            <div key={group.key} className="rounded-lg border border-slate-100 overflow-hidden">
              <button
                onClick={() => toggleGroup(group.key)}
                className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-slate-50 transition-colors"
              >
                <span className="text-[11px] font-medium text-slate-500 w-16 shrink-0">{group.label}</span>
                <span className="text-[11px] text-slate-700 font-medium truncate flex-1">
                  {currentItem?.name ?? '未配置'}
                </span>
                <span className="text-[10px] text-slate-300">{availableCount}/{group.items.length}</span>
                <ChevronDown size={12} className={`text-slate-400 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
              </button>

              {isExpanded && (
                <div className="border-t border-slate-100 bg-slate-50/50 py-1">
                  {group.items.map((item) => (
                    <button
                      key={item.name}
                      onClick={() => { group.onSelect(item.model || item.name); setExpandedGroup(null); }}
                      disabled={!item.available}
                      className={`flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors ${
                        item.available
                          ? (item.model || item.name) === group.selected || item.name === group.selected
                            ? 'bg-accent/10'
                            : 'hover:bg-white'
                          : 'opacity-40 cursor-not-allowed'
                      }`}
                    >
                      <Circle
                        size={6}
                        className={item.available ? 'text-emerald-400 fill-emerald-400' : 'text-slate-300 fill-slate-300'}
                      />
                      <span className={`text-[11px] ${(item.model || item.name) === group.selected || item.name === group.selected ? 'text-accent font-medium' : 'text-slate-600'}`}>
                        {item.name}
                      </span>
                      <span className="text-[10px] text-slate-400 ml-auto truncate max-w-[80px]">
                        {item.model}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

    </aside>
  );
}
