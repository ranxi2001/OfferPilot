import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import type { ToolDefinition } from '../types.js';

const DB_PATH = resolve('data/agent.db');

const MOCK_RESULTS = [
  {
    title: 'ReAct 循环的工程实现',
    dimension: 'architecture',
    question: '什么是 ReAct 模式？在工程实现中需要注意什么？',
    expertAnswer: 'ReAct 核心是 Observe → Think → Act → Observe 闭环。工程关键点：循环终止条件（max_iterations 兜底）、Tool 错误恢复、Context 膨胀管理、结构化日志观测。',
  },
  {
    title: 'Tool Calling 机制',
    dimension: 'architecture',
    question: 'Agent 的 Tool Calling 机制是怎么工作的？',
    expertAnswer: 'LLM 输出结构化调用意图而非自然语言。关键差异：OpenAI function_call 参数是字符串 JSON 需 parse，Anthropic tool_use 直接是 object。工程重点：schema 精度、流式拼接、并行策略。',
  },
  {
    title: 'Agent Harness vs 框架',
    dimension: 'engineering',
    question: '什么是 Agent Harness？和 LangChain 有什么区别？',
    expertAnswer: 'Harness 是自建基础设施层，10 层模型涵盖 Tools→Skills→QueryEngine→Context→Memory→Permission→Sessions→Command→Hook→Sub-agent。选择 Harness 而非框架是因为生产需要完全控制权、可调试性和性能定位。',
  },
  {
    title: 'System Prompt 设计',
    dimension: 'model',
    question: 'System Prompt 的设计有什么讲究？',
    expertAnswer: '5 个设计原则：角色具体、边界明确、格式规范、Tool 指引、分段组织。坑：太长稀释注意力、指令冲突、缺 negative examples。',
  },
  {
    title: '多 Provider 统一调用层',
    dimension: 'architecture',
    question: '如何设计一个支持多 Provider 的 LLM 调用层？',
    expertAnswer: '5 层设计：统一 Provider Interface（AsyncIterable<StreamEvent>）、Provider Router（model→provider 映射）、Retry + Error Classification（统一错误分类）、Token 计数归一化、流式适配层。',
  },
];

export const searchKnowledge: ToolDefinition = {
  schema: {
    name: 'search_knowledge',
    description: '搜索面试知识库，查找与指定主题相关的面试题目、参考答案和考察点分析',
    parameters: {
      type: 'object',
      properties: {
        query: { type: 'string', description: '搜索关键词或问题描述' },
        dimension: {
          type: 'string',
          enum: ['architecture', 'engineering', 'model', 'rag', 'multi-agent', 'evaluation', 'full-stack'],
          description: '限定搜索的维度分类（可选）',
        },
        limit: { type: 'number', description: '返回结果数量上限，默认 5' },
      },
      required: ['query'],
    },
  },
  riskLevel: 'low',
  async execute(input) {
    const { query, dimension, limit = 5 } = input as {
      query: string;
      dimension?: string;
      limit?: number;
    };

    // Try real database first
    if (existsSync(DB_PATH)) {
      try {
        const { openDatabase } = await import('../../db/database.js');
        const { KnowledgeSearch } = await import('../../knowledge/search.js');
        const db = openDatabase(DB_PATH);
        const search = new KnowledgeSearch(db);
        const results = search.search({ query, dimension, limit });
        db.close();
        return {
          success: true,
          output: JSON.stringify({
            query,
            dimension,
            results: results.map((r) => ({
              title: r.entry.title,
              dimension: r.entry.dimension,
              question: r.entry.question,
              expertAnswer: r.entry.expertAnswer?.slice(0, 300),
              score: r.score,
            })),
            source: 'database',
          }),
        };
      } catch (err) {
        console.warn(`[search_knowledge] DB error, falling back to mock: ${(err as Error).message}`);
      }
    }

    // Fallback to mock results
    const lower = query.toLowerCase();
    let filtered = MOCK_RESULTS;

    if (dimension) {
      filtered = filtered.filter((r) => r.dimension === dimension);
    }

    filtered = filtered.filter(
      (r) =>
        r.title.toLowerCase().includes(lower) ||
        r.question.toLowerCase().includes(lower) ||
        r.expertAnswer.toLowerCase().includes(lower),
    );

    return {
      success: true,
      output: JSON.stringify({
        query,
        dimension,
        results: filtered.slice(0, limit).map((r, i) => ({
          title: r.title,
          dimension: r.dimension,
          question: r.question,
          expertAnswer: r.expertAnswer,
          score: 1 - i * 0.1,
        })),
      }),
      metadata: { source: 'mock' },
    };
  },
};
