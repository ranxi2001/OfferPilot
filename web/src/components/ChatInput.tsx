'use client';

import { useState, useRef, useCallback } from 'react';
import { Mic, Paperclip, Send, Square } from 'lucide-react';

interface Props {
  onSend: (message: string) => void;
  onAudioAnswer?: (audio: Blob, fileName?: string) => void;
  disabled?: boolean;
  isTranscribing?: boolean;
}

export function ChatInput({ onSend, onAudioAnswer, disabled, isTranscribing }: Props) {
  const [input, setInput] = useState('');
  const [isRecording, setIsRecording] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const samplesRef = useRef<Float32Array[]>([]);
  const sampleRateRef = useRef(16000);

  const handleSubmit = useCallback(() => {
    if (!input.trim() || disabled || isTranscribing) return;
    onSend(input.trim());
    setInput('');
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
    }
  }, [input, disabled, isTranscribing, onSend]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  const handleInput = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value);
    const el = e.target;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 200) + 'px';
  };

  const handleAudioFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file && onAudioAnswer) {
      onAudioAnswer(file, file.name);
    }
    e.target.value = '';
  };

  const toggleRecording = async () => {
    if (isRecording) {
      stopRecording();
      return;
    }

    if (!onAudioAnswer || disabled || isTranscribing) return;

    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const AudioContextCtor = window.AudioContext || window.webkitAudioContext;
    const audioContext = new AudioContextCtor();
    const source = audioContext.createMediaStreamSource(stream);
    const processor = audioContext.createScriptProcessor(4096, 1, 1);

    samplesRef.current = [];
    sampleRateRef.current = audioContext.sampleRate;
    processor.onaudioprocess = (event) => {
      samplesRef.current.push(new Float32Array(event.inputBuffer.getChannelData(0)));
    };

    source.connect(processor);
    processor.connect(audioContext.destination);
    streamRef.current = stream;
    audioContextRef.current = audioContext;
    processorRef.current = processor;
    setIsRecording(true);
  };

  const stopRecording = () => {
    processorRef.current?.disconnect();
    streamRef.current?.getTracks().forEach((track) => track.stop());
    void audioContextRef.current?.close();

    processorRef.current = null;
    streamRef.current = null;
    audioContextRef.current = null;
    setIsRecording(false);

    const wav = encodeWav(samplesRef.current, sampleRateRef.current);
    onAudioAnswer?.(wav, `recording-${Date.now()}.wav`);
  };

  return (
    <div className="border-t border-zinc-800 bg-surface p-4">
      <div className="mx-auto flex max-w-3xl items-end gap-3">
        <input
          ref={fileRef}
          type="file"
          accept="audio/*"
          className="hidden"
          onChange={handleAudioFile}
        />
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          disabled={disabled || isTranscribing}
          title="上传录音"
          className="flex h-11 w-11 items-center justify-center rounded-xl border border-zinc-700 text-zinc-300 transition hover:border-primary hover:text-primary disabled:opacity-30"
        >
          <Paperclip size={18} />
        </button>
        <button
          type="button"
          onClick={toggleRecording}
          disabled={disabled || isTranscribing}
          title={isRecording ? '停止录音' : '开始录音'}
          className={`flex h-11 w-11 items-center justify-center rounded-xl border transition disabled:opacity-30 ${
            isRecording
              ? 'border-red-500 bg-red-500/15 text-red-300'
              : 'border-zinc-700 text-zinc-300 hover:border-primary hover:text-primary'
          }`}
        >
          {isRecording ? <Square size={16} /> : <Mic size={18} />}
        </button>
        <textarea
          ref={textareaRef}
          value={input}
          onChange={handleInput}
          onKeyDown={handleKeyDown}
          placeholder={isTranscribing ? '正在转写录音...' : '输入面试题或你的回答...'}
          disabled={disabled || isTranscribing}
          rows={1}
          className="flex-1 resize-none rounded-xl border border-zinc-700 bg-zinc-900 px-4 py-3 text-sm text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-primary disabled:opacity-50"
        />
        <button
          onClick={handleSubmit}
          disabled={disabled || isTranscribing || !input.trim()}
          className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary text-white transition hover:bg-primary-dark disabled:opacity-30"
        >
          <Send size={18} />
        </button>
      </div>
    </div>
  );
}

declare global {
  interface Window {
    webkitAudioContext?: typeof AudioContext;
  }
}

function encodeWav(chunks: Float32Array[], sampleRate: number): Blob {
  const length = chunks.reduce((total, chunk) => total + chunk.length, 0);
  const pcm = new Int16Array(length);
  let offset = 0;

  for (const chunk of chunks) {
    for (let i = 0; i < chunk.length; i++) {
      const sample = Math.max(-1, Math.min(1, chunk[i]));
      pcm[offset++] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
    }
  }

  const buffer = new ArrayBuffer(44 + pcm.length * 2);
  const view = new DataView(buffer);
  writeString(view, 0, 'RIFF');
  view.setUint32(4, 36 + pcm.length * 2, true);
  writeString(view, 8, 'WAVE');
  writeString(view, 12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeString(view, 36, 'data');
  view.setUint32(40, pcm.length * 2, true);

  let byteOffset = 44;
  for (let i = 0; i < pcm.length; i++) {
    view.setInt16(byteOffset, pcm[i], true);
    byteOffset += 2;
  }

  return new Blob([buffer], { type: 'audio/wav' });
}

function writeString(view: DataView, offset: number, value: string): void {
  for (let i = 0; i < value.length; i++) {
    view.setUint8(offset + i, value.charCodeAt(i));
  }
}
