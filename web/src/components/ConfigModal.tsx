'use client';

import { useState, useEffect } from 'react';
import { X, Save, Eye, EyeOff, Circle } from 'lucide-react';

interface ModelItem {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  env_key: string;
  model_env_key: string | null;
  base_url_env_key: string | null;
  available: boolean;
}

interface ConfigData {
  text: ModelItem[];
  tts: ModelItem[];
  multimodal: ModelItem[];
  envVars: Record<string, string>;
}

interface Props {
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

const GROUP_LABELS: Record<string, { label: string; desc: string }> = {
  text: { label: '文本模型', desc: '面试诊断、简历分析使用' },
  tts: { label: 'TTS 语音合成', desc: '模拟面试语音播报' },
  multimodal: { label: '语音识别 / 多模态', desc: '录音转写使用' },
};

export function ConfigModal({ open, onClose, onSaved }: Props) {
  const [config, setConfig] = useState<ConfigData | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [visibleKeys, setVisibleKeys] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState(false);
  const [activeTab, setActiveTab] = useState<string>('text');
  const [status, setStatus] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    fetch('/api/config')
      .then((r) => r.json())
      .then((data: ConfigData) => {
        setConfig(data);
        setValues(data.envVars);
      })
      .catch(() => {});
  }, [open]);

  if (!open) return null;

  const handleSave = async () => {
    setSaving(true);
    setStatus(null);
    try {
      const res = await fetch('/api/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ envVars: values }),
      });
      if (!res.ok) throw new Error('Save failed');
      setStatus('已保存到 .env');
      onSaved();
      setTimeout(() => setStatus(null), 2000);
    } catch {
      setStatus('保存失败');
    } finally {
      setSaving(false);
    }
  };

  const toggleVisible = (key: string) => {
    const next = new Set(visibleKeys);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    setVisibleKeys(next);
  };

  const updateValue = (key: string, val: string) => {
    setValues((prev) => ({ ...prev, [key]: val }));
  };

  const groups = ['text', 'tts', 'multimodal'] as const;
  const currentModels = config?.[activeTab as keyof Pick<ConfigData, 'text' | 'tts' | 'multimodal'>] ?? [];
  const currentMeta = GROUP_LABELS[activeTab];

  const getModelEnvKeys = (item: ModelItem): { key: string; label: string; isSecret: boolean }[] => {
    const keys: { key: string; label: string; isSecret: boolean }[] = [];
    keys.push({ key: item.env_key, label: 'API Key', isSecret: true });
    if (item.base_url_env_key) {
      keys.push({ key: item.base_url_env_key, label: 'Base URL', isSecret: false });
    }
    if (item.model_env_key) {
      keys.push({ key: item.model_env_key, label: 'Model', isSecret: false });
    }
    return keys;
  };

  // Deduplicate keys across models in same group (e.g., OPENAI_API_KEY shared)
  const seenKeys = new Set<string>();

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={onClose} />
      <div className="relative w-full max-w-2xl max-h-[85vh] rounded-2xl bg-white shadow-2xl flex flex-col overflow-hidden mx-4">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-100">
          <div>
            <h2 className="text-base font-semibold text-primary">模型配置</h2>
            <p className="text-xs text-slate-400 mt-0.5">编辑后保存至项目 .env，模型定义见 models.yml</p>
          </div>
          <button onClick={onClose} className="p-1.5 rounded-lg hover:bg-slate-100 transition-colors">
            <X size={18} className="text-slate-400" />
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-slate-100 px-6">
          {groups.map((g) => (
            <button
              key={g}
              onClick={() => { setActiveTab(g); seenKeys.clear(); }}
              className={`px-4 py-2.5 text-xs font-medium border-b-2 transition-colors ${
                activeTab === g
                  ? 'border-accent text-accent'
                  : 'border-transparent text-slate-500 hover:text-slate-700'
              }`}
            >
              {GROUP_LABELS[g].label}
            </button>
          ))}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto px-6 py-4 space-y-5">
          <p className="text-[11px] text-slate-400">{currentMeta.desc}</p>

          {currentModels.map((item) => {
            const envKeys = getModelEnvKeys(item);

            return (
              <div key={item.name} className="space-y-2.5">
                <div className="flex items-center gap-2">
                  <Circle
                    size={7}
                    className={item.available ? 'text-emerald-400 fill-emerald-400' : 'text-slate-300 fill-slate-300'}
                  />
                  <span className="text-xs font-semibold text-slate-700">{item.name}</span>
                  {item.model && !item.model.includes('${') && (
                    <span className="text-[10px] text-slate-400 ml-auto">{item.model}</span>
                  )}
                </div>

                {envKeys.map((field) => {
                  if (seenKeys.has(field.key)) return null;
                  seenKeys.add(field.key);
                  return (
                    <div key={field.key} className="flex items-center gap-2">
                      <label className="text-[11px] text-slate-500 w-20 shrink-0 text-right">
                        {field.label}
                      </label>
                      <div className="flex-1 flex items-center gap-1 rounded-lg border border-slate-200 bg-surface-muted px-3 py-1.5">
                        <input
                          type={field.isSecret && !visibleKeys.has(field.key) ? 'password' : 'text'}
                          value={values[field.key] ?? ''}
                          onChange={(e) => updateValue(field.key, e.target.value)}
                          placeholder={field.key}
                          className="flex-1 bg-transparent text-xs text-slate-700 placeholder-slate-300 outline-none"
                        />
                        {field.isSecret && (
                          <button
                            onClick={() => toggleVisible(field.key)}
                            className="p-0.5 text-slate-400 hover:text-slate-600"
                          >
                            {visibleKeys.has(field.key) ? <EyeOff size={12} /> : <Eye size={12} />}
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })}

          {currentModels.length === 0 && (
            <p className="text-xs text-slate-400 text-center py-8">
              models.yml 中未配置此分类的模型
            </p>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-3 border-t border-slate-100 bg-slate-50/50">
          <span className="text-[11px] text-slate-400">
            {status ?? '配置存储于项目根目录 .env'}
          </span>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-4 py-2 rounded-lg text-xs font-medium text-slate-600 hover:bg-slate-100 transition-colors"
            >
              取消
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-1.5 px-4 py-2 rounded-lg bg-accent text-xs font-medium text-white hover:bg-accent-dark disabled:opacity-50 transition-colors"
            >
              <Save size={12} />
              {saving ? '保存中...' : '保存'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
