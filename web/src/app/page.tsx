'use client';

import { useState, useRef, useEffect } from 'react';
import { ChatMessage, type Message } from '@/components/ChatMessage';
import { ChatInput } from '@/components/ChatInput';
import { Sidebar, type ViewType } from '@/components/Sidebar';
import { Header } from '@/components/Header';
import { InterviewView } from '@/components/InterviewView';
import { ResumeView } from '@/components/ResumeView';
import { MatchView } from '@/components/MatchView';
import { DashboardView } from '@/components/DashboardView';
import { ExportButton } from '@/components/ExportButton';
import { ConfigModal } from '@/components/ConfigModal';
import { Compass, MessageSquare, FileText, BarChart3 } from 'lucide-react';

export default function Home() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [isThinking, setIsThinking] = useState(false);
  const [isTranscribing, setIsTranscribing] = useState(false);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [model, setModel] = useState('claude');
  const [activeView, setActiveView] = useState<ViewType>('chat');
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
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
    setIsThinking(false);
    const assistantId = (Date.now() + 1).toString();
    setMessages((prev) => [...prev, { id: assistantId, role: 'assistant', content: '', thinking: '' }]);

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

            if (event.type === 'thinking_delta') {
              setIsThinking(true);
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId
                    ? { ...m, thinking: (m.thinking ?? '') + event.content }
                    : m,
                ),
              );
            } else if (event.type === 'text_delta') {
              setIsThinking(false);
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
      setIsThinking(false);
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
      if (!response.ok) throw new Error(data.error ?? '转写失败');

      const transcript = data.text as string;
      setMessages((prev) =>
        prev.map((m) =>
          m.id === pendingId
            ? { ...m, content: '录音转写完成，正在生成诊断。', status: '诊断中', transcript }
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
            ? { ...m, content: `录音转写失败: ${(err as Error).message}`, isError: true, status: '失败' }
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
      {/* Mobile sidebar overlay */}
      {mobileMenuOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div className="absolute inset-0 bg-black/20 backdrop-blur-sm" onClick={() => setMobileMenuOpen(false)} />
          <div className="absolute left-0 top-0 h-full w-72 animate-slide-up">
            <Sidebar
              activeView={activeView}
              onViewChange={(v) => { setActiveView(v); setMobileMenuOpen(false); }}
              model={model}
              onModelChange={setModel}
              onOpenConfig={() => { setMobileMenuOpen(false); setConfigOpen(true); }}
            />
          </div>
        </div>
      )}

      <Sidebar
        activeView={activeView}
        onViewChange={setActiveView}
        model={model}
        onModelChange={setModel}
        onOpenConfig={() => setConfigOpen(true)}
      />

      <main className="flex flex-1 flex-col min-w-0">
        <Header
          activeView={activeView}
          sessionId={sessionId}
          questionsCount={messages.filter((m) => m.role === 'user').length}
          onReset={handleReset}
          onMobileMenu={() => setMobileMenuOpen(true)}
        />
        {activeView === 'chat' && messages.length > 0 && (
          <div className="flex justify-end px-5 pt-2">
            <ExportButton messages={messages} sessionId={sessionId} />
          </div>
        )}

        {activeView === 'chat' && (
          <>
            <div className="flex-1 overflow-y-auto px-4 py-6">
              <div className="mx-auto max-w-3xl space-y-5">
                {messages.length === 0 && <WelcomeScreen onSend={sendMessage} />}
                {messages.map((msg, i) => (
                  <ChatMessage
                    key={msg.id}
                    message={msg}
                    isStreaming={isStreaming && i === messages.length - 1 && msg.role === 'assistant'}
                    isThinking={isThinking && i === messages.length - 1 && msg.role === 'assistant'}
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
          </>
        )}

        {activeView === 'interview' && <InterviewView />}
        {activeView === 'resume' && <ResumeView />}
        {activeView === 'match' && <MatchView />}
        {activeView === 'dashboard' && <DashboardView />}
      </main>

      <ConfigModal open={configOpen} onClose={() => setConfigOpen(false)} onSaved={() => {}} />
    </div>
  );
}

function WelcomeScreen({ onSend }: { onSend: (msg: string) => void }) {
  const suggestions = [
    { text: '诊断一个 Agent 架构面试回答', icon: MessageSquare },
    { text: '帮我分析这个面试题的考察重点', icon: Compass },
    { text: '模拟一场 RAG 方向面试', icon: BarChart3 },
    { text: '我的回答有哪些可以改进的地方', icon: FileText },
  ];

  return (
    <div className="flex flex-col items-center justify-center py-16 text-center animate-fade-in">
      <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-accent to-cyan shadow-elevated">
        <Compass size={28} className="text-white" />
      </div>

      <h1 className="mb-2 text-2xl font-bold text-primary">面试诊断 Agent</h1>
      <p className="mb-8 max-w-md text-sm text-slate-500 leading-relaxed">
        输入面试题和你的回答，获取 AI 驱动的结构化诊断。<br />
        支持 7 大维度评估、追问建议、专家级参考答案。
      </p>

      <div className="grid w-full max-w-lg grid-cols-1 gap-2.5 sm:grid-cols-2">
        {suggestions.map((item) => {
          const Icon = item.icon;
          return (
            <button
              key={item.text}
              onClick={() => onSend(item.text)}
              className="group flex items-center gap-3 rounded-xl border border-slate-200 bg-white px-4 py-3 text-left shadow-card hover:border-accent/40 hover:shadow-glow transition-all"
            >
              <Icon size={14} className="text-slate-400 group-hover:text-accent shrink-0" />
              <span className="text-xs text-slate-600 group-hover:text-primary">{item.text}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
