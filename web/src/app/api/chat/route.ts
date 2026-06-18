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
        await sleep(200);

        const thinking = generateThinking(message);
        const thinkingChunks = chunkText(thinking);
        for (const chunk of thinkingChunks) {
          send({ type: 'thinking_delta', content: chunk });
          await sleep(15);
        }

        await sleep(300);

        send({ type: 'tool_call', name: 'search_knowledge' });
        await sleep(400);

        const response = generateMockResponse(message);
        const chunks = chunkText(response);
        for (const chunk of chunks) {
          send({ type: 'text_delta', content: chunk });
          await sleep(25);
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

function generateThinking(input: string): string {
  const lower = input.toLowerCase();

  if (lower.includes('react') || lower.includes('agent') || lower.includes('循环')) {
    return '用户问的是关于 Agent/ReAct 循环的面试题。我需要从知识库检索相关内容，分析回答的完整度。关键考察点：1) 是否提到 Reasoning-Acting 交替 2) 是否有工程化细节（token 预算、abort 机制、并行 tool）3) 是否涉及生产环境踩坑。让我先搜索知识库中 architecture 维度的内容...';
  }

  if (lower.includes('rag') || lower.includes('检索')) {
    return '这是一个 RAG 方向的面试问题。我需要评估候选人对检索增强生成的理解深度。核心维度：Chunk 策略选择、Embedding 模型对比、混合检索（BM25 + 向量）、Rerank 机制、上下文窗口管理。让我检索知识库...';
  }

  if (lower.includes('简历') || lower.includes('resume')) {
    return '用户需要简历相关的诊断。我会从 STAR 结构完整度、技术栈量化、项目影响力描述三个维度进行分析...';
  }

  return '分析用户输入，确定考察维度和诊断方向。让我从知识库中检索最相关的内容来对比评估...';
}

function generateMockResponse(input: string): string {
  const lower = input.toLowerCase();

  if (lower.includes('你好') || lower.includes('hello')) {
    return `## 你好！\n\n我是 **OfferPilot 面试诊断 Agent**，专注于 AI Agent / LLM 工程方向的面试辅导。\n\n### 我能帮你做什么：\n\n| 功能 | 说明 |\n|:---|:---|\n| 🎯 诊断回答 | 输入面试题 + 回答，获取评分与改进建议 |\n| 🔍 分析考点 | 拆解面试题的考察维度和得分点 |\n| 🎲 模拟面试 | 按方向和难度生成追问序列 |\n| 📝 优化表达 | 帮你重写回答，结构化 + 加深度 |\n\n试试输入一个面试问题开始吧！`;
  }

  if (lower.includes('react') || lower.includes('agent') || lower.includes('循环') || lower.includes('架构')) {
    return `## 诊断结果\n\n### 评分：6.5 / 10\n\n---\n\n### 💡 考察维度：架构设计\n\n你的回答触及了核心概念（思考-行动循环），但缺少 **工程化深度**。\n\n### ✅ 亮点\n- 提到了 Reasoning + Acting 的基本框架\n- 方向正确，说明理解了核心思想\n\n### ⚠️ 差距\n| 方面 | 你的回答 | 专家水平 |\n|:---|:---|:---|\n| 工程实现 | 未提及 | Token budget、abort 机制、并行 tool 执行 |\n| 错误处理 | 未提及 | 工具调用失败重试、max iterations 兜底 |\n| 生产经验 | 缺少 | 上下文压缩、流式输出、使用量统计 |\n\n### 📌 改进建议\n\n1. **先定义后展开**：\"ReAct 循环是 Agent 的核心执行模式，由 Reasoning（推理）→ Action（工具调用）→ Observation（结果观察）三步循环组成\"\n2. **加工程细节**：提到 token budget 控制、并行 tool 执行、abort signal 等生产级特性\n3. **举实例**：\"比如我们的 Agent 最多迭代 10 轮，每轮先检查 token 余量...\"\n\n### 🎯 追问预测\n> \"如果 Agent 陷入循环怎么办？如何优雅退出？\"`;
  }

  if (lower.includes('rag') || lower.includes('检索')) {
    return `## 诊断结果\n\n### 评分：7.0 / 10\n\n---\n\n### 💡 考察维度：RAG 检索\n\n### ✅ 亮点\n- 理解 RAG 的基本架构\n- 知道 embedding 的作用\n\n### ⚠️ 需要补充\n1. **Chunk 策略**：固定长度 vs 语义切分 vs 递归切分的取舍\n2. **混合检索**：BM25 + 向量双通道 + Rerank 排序\n3. **评测指标**：Hit Rate、MRR、NDCG\n\n### 📌 专家级回答参考\n\n> RAG 的核心挑战不是 \"能不能检索到\"，而是 \"检索到的是否是最相关的\"。我们用 BM25 做初筛保证召回率，embedding 做精排保证相关性，最后用 Cross-encoder Rerank 做最终排序...`;
  }

  if (lower.includes('模拟') || lower.includes('面试')) {
    return `## 🎲 模拟面试启动\n\n**方向**：AI Agent 工程\n**难度**：中高级\n\n---\n\n### 第 1 题\n\n> 请描述你对 Agent 系统中 \"上下文管理\" 的理解。在 context window 有限的情况下，如何保证 Agent 在多轮对话中不丢失关键信息？\n\n**考察点**：\n- 上下文压缩策略\n- 摘要 vs 截断 vs 滑动窗口\n- 实体提取与记忆分离\n\n---\n\n请回答后，我会给出诊断和追问。`;
  }

  return `## 收到！\n\n请用以下格式输入：\n\n> **题目**：（面试问题）\n> **我的回答**：（你的作答）\n\n我会给出：\n- 📊 评分（1-10）\n- 🔍 差距分析\n- 💡 改进建议\n- 🎯 追问预测\n\n或者你也可以直接说 \"模拟一场面试\"，我来出题。`;
}

function chunkText(text: string): string[] {
  const chunks: string[] = [];
  let i = 0;
  while (i < text.length) {
    const size = 2 + Math.floor(Math.random() * 6);
    chunks.push(text.slice(i, i + size));
    i += size;
  }
  return chunks;
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
