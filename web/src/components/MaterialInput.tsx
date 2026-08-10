'use client';

import { useRef, useState } from 'react';
import { AlertCircle, FileText, Link2, Loader2, Type, Upload, X } from 'lucide-react';
import type { InterviewMaterial } from '@/types/interview';

const ACCEPTED_EXTENSIONS = ['.pdf', '.docx', '.md', '.txt', '.tex'];

interface MaterialInputProps {
  label: string;
  description: string;
  emptyName: string;
  value: InterviewMaterial | null;
  onChange: (material: InterviewMaterial | null) => void;
}

type InputMode = 'upload' | 'paste' | 'url';

export function MaterialInput({ label, description, emptyName, value, onChange }: MaterialInputProps) {
  const [mode, setMode] = useState<InputMode>('upload');
  const [url, setUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  async function parseFile(file: File) {
    const extension = `.${file.name.toLowerCase().split('.').pop()}`;
    if (!ACCEPTED_EXTENSIONS.includes(extension)) {
      setError(`支持 ${ACCEPTED_EXTENSIONS.join(' / ')}`);
      return;
    }

    setLoading(true);
    setError(null);
    try {
      if (extension === '.txt' || extension === '.md') {
        const text = await file.text();
        onChange({ name: file.name, text, source: 'upload', mimeType: file.type });
        return;
      }
      const form = new FormData();
      form.append('file', file);
      const response = await fetch('/api/parse-pdf', { method: 'POST', body: form });
      const data = await response.json() as { text?: string; error?: string };
      if (!response.ok || !data.text?.trim()) throw new Error(data.error || '没有提取到文本');
      onChange({ name: file.name, text: data.text, source: 'upload', mimeType: file.type });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function parseUrl() {
    if (!url.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const response = await fetch('/api/parse-url', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      });
      const data = await response.json() as { text?: string; title?: string; error?: string };
      if (!response.ok || !data.text?.trim()) throw new Error(data.error || '没有提取到网页正文');
      onChange({ name: data.title || url, text: data.text, source: 'url', url });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="min-w-0 rounded-lg border border-slate-200 bg-white p-4 shadow-card">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-primary">{label}</h3>
          <p className="mt-0.5 text-xs text-slate-400">{description}</p>
        </div>
        {value && (
          <button
            type="button"
            title={`清除${label}`}
            onClick={() => onChange(null)}
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-slate-400 hover:bg-red-50 hover:text-red-500"
          >
            <X size={15} />
          </button>
        )}
      </div>

      {!value && (
        <div className="mb-3 grid grid-cols-3 gap-1 rounded-md bg-slate-100 p-1">
          {([
            ['upload', '上传', Upload],
            ['paste', '粘贴', Type],
            ['url', '网址', Link2],
          ] as const).map(([id, text, Icon]) => (
            <button
              key={id}
              type="button"
              onClick={() => { setMode(id); setError(null); }}
              className={`flex h-8 items-center justify-center gap-1.5 rounded px-2 text-xs font-medium ${
                mode === id ? 'bg-white text-primary shadow-sm' : 'text-slate-500 hover:text-slate-700'
              }`}
            >
              <Icon size={13} />
              {text}
            </button>
          ))}
        </div>
      )}

      {loading ? (
        <div className="flex h-36 items-center justify-center gap-2 text-xs text-slate-500">
          <Loader2 size={16} className="animate-spin text-accent" />
          正在提取文本
        </div>
      ) : value ? (
        <div>
          <div className="mb-2 flex items-center gap-2 rounded-md bg-slate-50 px-3 py-2">
            <FileText size={14} className="shrink-0 text-accent" />
            <span className="min-w-0 flex-1 truncate text-xs font-medium text-slate-600">{value.name}</span>
            <span className="shrink-0 text-[11px] text-slate-400">{value.text.length} 字</span>
          </div>
          <textarea
            value={value.text}
            onChange={(event) => onChange({ ...value, text: event.target.value })}
            rows={7}
            className="w-full resize-none rounded-md border border-slate-200 bg-white px-3 py-2.5 text-xs leading-5 text-slate-700 outline-none focus:border-accent/60 focus:ring-2 focus:ring-accent/10"
          />
        </div>
      ) : mode === 'upload' ? (
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          onDragOver={(event) => event.preventDefault()}
          onDrop={(event) => {
            event.preventDefault();
            const file = event.dataTransfer.files[0];
            if (file) void parseFile(file);
          }}
          className="flex h-36 w-full flex-col items-center justify-center gap-2 rounded-md border border-dashed border-slate-300 bg-slate-50 text-slate-500 hover:border-accent/60 hover:bg-sky-50/50"
        >
          <Upload size={20} className="text-accent" />
          <span className="text-xs font-medium">拖入文件或点击选择</span>
          <span className="text-[11px] text-slate-400">PDF / DOCX / MD / TXT / TEX</span>
          <input
            ref={fileRef}
            type="file"
            accept={ACCEPTED_EXTENSIONS.join(',')}
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = '';
              if (file) void parseFile(file);
            }}
          />
        </button>
      ) : mode === 'paste' ? (
        <textarea
          autoFocus
          rows={7}
          placeholder={`粘贴${label}正文`}
          onChange={(event) => {
            const text = event.target.value;
            onChange(text ? { name: emptyName, text, source: 'paste' } : null);
          }}
          className="w-full resize-none rounded-md border border-slate-200 bg-white px-3 py-2.5 text-xs leading-5 text-slate-700 outline-none focus:border-accent/60 focus:ring-2 focus:ring-accent/10"
        />
      ) : (
        <div className="flex h-36 flex-col justify-center gap-2">
          <div className="flex items-center gap-2">
            <div className="flex h-10 min-w-0 flex-1 items-center gap-2 rounded-md border border-slate-200 px-3">
              <Link2 size={14} className="shrink-0 text-slate-400" />
              <input
                type="url"
                value={url}
                onChange={(event) => setUrl(event.target.value)}
                onKeyDown={(event) => { if (event.key === 'Enter') void parseUrl(); }}
                placeholder="https://..."
                className="min-w-0 flex-1 bg-transparent text-xs text-slate-700 outline-none"
              />
            </div>
            <button
              type="button"
              title="抓取网页正文"
              onClick={() => void parseUrl()}
              disabled={!url.trim()}
              className="flex h-10 w-10 items-center justify-center rounded-md bg-accent text-white disabled:opacity-40"
            >
              <Link2 size={15} />
            </button>
          </div>
        </div>
      )}

      {error && (
        <div className="mt-2 flex items-start gap-2 rounded-md bg-red-50 px-3 py-2 text-xs text-red-600">
          <AlertCircle size={13} className="mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}
    </section>
  );
}
