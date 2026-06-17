'use client';

import { useState, useRef, useEffect } from 'react';
import { ChatMessage, type Message } from '@/components/ChatMessage';
import { ChatInput } from '@/components/ChatInput';
import { Sidebar } from '@/components/Sidebar';
import { Header } from '@/components/Header';

export default function Home() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [isTranscribing, setIsTranscribing] = useState(false);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [model, setModel] = useState('gpt-5.5');
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => {
    createSession();
  }, []);

  async function createSession() {
    try {
      const res = await fetch('/api/session', { method: 'POST' });
      const data = await res.json();
      setSessionId(data.sessionId);
    } catch {
      setSessionId('local-' + Date.now());
    }
  }

  async function sendMessage(content: string, opts?: { showUserMessage?: boolean }) {
    if (!content.trim() || isStreaming) return;

    if (opts?.showUserMessage !== false) {
      const userMsg: Message = { id: Date.now().toString(), role: 'user', content };
      setMessages((prev) => [...prev, userMsg]);
    }

    setIsStreaming(true);
    const assistantId = (Date.now() + 1).toString();
    setMessages((prev) => [...prev, { id: assistantId, role: 'assistant', content: '' }]);

    try {
      const response = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: content, sessionId, model }),
      });

      if (!response.body) throw new Error('No response body');

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;

          const data = line.slice(6);
          if (data === '[DONE]') continue;

          try {
            const event = JSON.parse(data);
            if (event.type === 'text_delta') {
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId ? { ...m, content: m.content + event.content } : m,
                ),
              );
            } else if (event.type === 'tool_call') {
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId
                    ? { ...m, toolCalls: [...(m.toolCalls ?? []), event.name] }
                    : m,
                ),
              );
            }
          } catch {}
        }
      }
    } catch (err) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantId
            ? { ...m, content: `错误: ${(err as Error).message}`, isError: true }
            : m,
        ),
      );
    } finally {
      setIsStreaming(false);
    }
  }

  async function handleAudioAnswer(audio: Blob, fileName?: string) {
    if (isStreaming || isTranscribing) return;

    setIsTranscribing(true);
    const pendingId = Date.now().toString();
    const displayName = fileName ?? `recording-${Date.now()}.wav`;
    const audioUrl = URL.createObjectURL(audio);

    setMessages((prev) => [
      ...prev,
      {
        id: pendingId,
        role: 'process',
        content: '录音已保存，正在转写并准备诊断。',
        status: '转写中',
        audioUrl,
        audioName: displayName,
      },
    ]);

    try {
      const response = await fetch('/api/transcribe', {
        method: 'POST',
        headers: {
          'Content-Type': audio.type || 'audio/wav',
          'X-File-Name': displayName,
        },
        body: audio,
      });

      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error ?? '转写失败');
      }

      const transcript = data.text as string;
      setMessages((prev) =>
        prev.map((m) =>
          m.id === pendingId
            ? {
                ...m,
                content: '录音转写完成，正在生成诊断。',
                status: '诊断中',
                transcript,
              }
            : m,
        ),
      );

      await sendMessage(`请诊断我的这段录音回答：\n\n${transcript}`, { showUserMessage: false });

      setMessages((prev) =>
        prev.map((m) =>
          m.id === pendingId ? { ...m, content: '录音诊断完成。', status: '完成' } : m,
        ),
      );
    } catch (err) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === pendingId
            ? {
                ...m,
                content: `录音转写失败: ${(err as Error).message}`,
                isError: true,
                status: '失败',
              }
            : m,
        ),
      );
    } finally {
      setIsTranscribing(false);
    }
  }

  function handleReset() {
    setMessages([]);
    createSession();
  }

  return (
    <div className="flex h-screen">
      <Sidebar onReset={handleReset} model={model} onModelChange={setModel} />
      <main className="flex flex-1 flex-col">
        <Header sessionId={sessionId} questionsCount={messages.filter((m) => m.role === 'user').length} />
        <div className="flex-1 overflow-y-auto px-4 py-6">
          <div className="mx-auto max-w-3xl space-y-4">
            {messages.length === 0 && <WelcomeScreen />}
            {messages.map((msg) => (
              <ChatMessage
                key={msg.id}
                message={msg}
                isStreaming={isStreaming && msg === messages[messages.length - 1]}
              />
            ))}
            <div ref={messagesEndRef} />
          </div>
        </div>
        <ChatInput
          onSend={sendMessage}
          onAudioAnswer={handleAudioAnswer}
          disabled={isStreaming}
          isTranscribing={isTranscribing}
        />
      </main>
    </div>
  );
}

function WelcomeScreen() {
  return (
    <div className="flex flex-col items-center justify-center py-20 text-center">
      <h1 className="mb-4 text-3xl font-bold text-primary">面试诊断 Agent</h1>
      <p className="mb-8 max-w-md text-zinc-400">
        输入面试题和你的回答，或直接录音上传，获取结构化诊断、追问建议和复盘文档。
      </p>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {[
          '帮我诊断一个 ReAct 循环的回答',
          '列出所有考察维度',
          '模拟一场 Agent 架构面试',
          '我的薄弱点在哪里',
        ].map((text) => (
          <button
            key={text}
            onClick={() => void 0}
            className="rounded-lg border border-zinc-700 px-4 py-3 text-left text-sm text-zinc-300 transition hover:border-primary hover:bg-surface-light"
          >
            {text}
          </button>
        ))}
      </div>
    </div>
  );
}
