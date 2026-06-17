import { NextRequest, NextResponse } from 'next/server';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:3001';
const API_KEY = process.env.OFFERPILOT_API_KEY;
const HEARTBEAT_INTERVAL = 15000;

function validateAuth(req: NextRequest): boolean {
  if (!API_KEY) return true;
  const authHeader = req.headers.get('authorization');
  if (!authHeader) return false;
  const token = authHeader.replace(/^Bearer\s+/i, '');
  return token === API_KEY;
}

export async function POST(req: NextRequest) {
  if (!validateAuth(req)) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 });
  }

  const body = await req.json();
  const { message, sessionId, model } = body as { message?: string; sessionId?: string; model?: string };

  if (!message) {
    return NextResponse.json({ error: 'message is required' }, { status: 400 });
  }

  const useMock = !process.env.BACKEND_URL;
  if (useMock) {
    return mockSSEResponse(body);
  }

  const backendRes = await fetch(`${BACKEND_URL}/api/chat`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
    },
    body: JSON.stringify({ message, sessionId, model }),
  });

  if (!backendRes.ok) {
    return NextResponse.json(
      { error: `Backend error: ${backendRes.status}` },
      { status: backendRes.status },
    );
  }

  return new Response(backendRes.body, {
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    },
  });
}

function mockSSEResponse(body: { message: string; model?: string }) {
  const encoder = new TextEncoder();
  const message = body.message ?? '';

  const stream = new ReadableStream({
    async start(controller) {
      const send = (event: Record<string, unknown>) => {
        controller.enqueue(encoder.encode(`data: ${JSON.stringify(event)}\n\n`));
      };

      const heartbeat = setInterval(() => {
        try {
          controller.enqueue(encoder.encode(`: ping\n\n`));
        } catch {
          clearInterval(heartbeat);
        }
      }, HEARTBEAT_INTERVAL);

      try {
        await sleep(100);

        const response = generateMockResponse(message);
        const chunks = chunkText(response);
        for (const chunk of chunks) {
          send({ type: 'text_delta', content: chunk });
          await sleep(30);
        }

        send({ type: 'done' });
      } finally {
        clearInterval(heartbeat);
        controller.enqueue(encoder.encode('data: [DONE]\n\n'));
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    },
  });
}

function generateMockResponse(input: string): string {
  const lower = input.toLowerCase();

  if (lower.includes('你好') || lower.includes('hello')) {
    return `你好！我是面试诊断 Agent，专注于 AI Agent / LLM 工程方向的面试辅导。\n\n你可以：\n1. 输入面试题 + 你的回答，我会给出诊断\n2. 问我任何 Agent 相关的面试问题\n3. 让我模拟面试官追问`;
  }

  if (lower.includes('react') || lower.includes('agent') || lower.includes('循环')) {
    return `## 诊断结果\n\n### 评分：6.5 / 10\n\n你的回答触及了核心概念，但缺少工程细节。\n\n**改进建议**：\n1. 先给概念定义，再展开工程要点\n2. 提到生产环境踩坑经验\n3. 用具体数字增强说服力`;
  }

  return `收到！请用以下格式输入：\n\n**题目**：（面试问题）\n**我的回答**：（你的作答）\n\n我会给出评分、差距分析和改进建议。`;
}

function chunkText(text: string): string[] {
  const chunks: string[] = [];
  let i = 0;
  while (i < text.length) {
    const size = 3 + Math.floor(Math.random() * 8);
    chunks.push(text.slice(i, i + size));
    i += size;
  }
  return chunks;
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
