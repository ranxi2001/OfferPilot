import type { ToolDefinition } from '../types.js';

export const mockInterview: ToolDefinition = {
  schema: {
    name: 'mock_interview',
    description: '根据 JD 和简历生成模拟面试题目序列，覆盖技术深度、项目经验、行为面试三个维度',
    parameters: {
      type: 'object',
      properties: {
        jdText: { type: 'string', description: 'JD 内容（可选）' },
        resumeText: { type: 'string', description: '简历内容（可选）' },
        dimension: {
          type: 'string',
          enum: ['technical', 'project', 'behavioral', 'mixed'],
          description: '面试维度（默认 mixed）',
        },
        difficulty: {
          type: 'string',
          enum: ['easy', 'medium', 'hard'],
          description: '难度（默认 medium）',
        },
        count: { type: 'number', description: '题目数量（默认 5）' },
      },
    },
  },
  riskLevel: 'low',
  async execute(input) {
    const { jdText, resumeText, dimension = 'mixed', difficulty = 'medium', count = 5 } = input as {
      jdText?: string;
      resumeText?: string;
      dimension?: string;
      difficulty?: string;
      count?: number;
    };

    const technicalQuestions = [
      { q: '请介绍一下 Agent 的 ReAct 循环，以及在工程实现中需要注意什么？', dim: 'technical', diff: 'medium' },
      { q: '如何设计一个支持多 Provider 的 LLM 调用层？说说你的接口抽象思路', dim: 'technical', diff: 'hard' },
      { q: 'RAG 系统中，Chunk 策略和 Retrieval 策略分别有哪些选择？trade-off 是什么？', dim: 'technical', diff: 'hard' },
      { q: 'Tool Calling 的流式处理要注意什么？如果 tool input 是增量送达的怎么处理？', dim: 'technical', diff: 'medium' },
      { q: '什么是 Agent Harness？和 LangChain 的本质区别在哪里？', dim: 'technical', diff: 'medium' },
      { q: '如何解决 Agent 循环中的 Context Window 膨胀问题？', dim: 'technical', diff: 'hard' },
      { q: 'Embedding 模型选型时你会考虑哪些因素？', dim: 'technical', diff: 'easy' },
      { q: 'System Prompt 的设计有什么讲究？怎么减少 prompt injection 风险？', dim: 'technical', diff: 'medium' },
    ];

    const projectQuestions = [
      { q: '你做过的最复杂的 Agent 项目是什么？遇到了什么核心难点？', dim: 'project', diff: 'medium' },
      { q: '你在项目中是如何做技术选型的？举一个关键决策的例子', dim: 'project', diff: 'medium' },
      { q: '说一个你优化系统性能的案例，量化结果是什么？', dim: 'project', diff: 'medium' },
      { q: '你如何衡量 Agent 的输出质量？用过什么评测方案？', dim: 'project', diff: 'hard' },
      { q: '项目中遇到过线上事故吗？你是怎么处理和复盘的？', dim: 'project', diff: 'medium' },
    ];

    const behavioralQuestions = [
      { q: '说一个你和团队意见不一致的例子，最终怎么解决的？', dim: 'behavioral', diff: 'medium' },
      { q: '你是怎么在紧急 deadline 下保证交付质量的？', dim: 'behavioral', diff: 'easy' },
      { q: '你是怎么快速学习一个新技术领域的？举个最近的例子', dim: 'behavioral', diff: 'easy' },
      { q: '你如何评估自己的技术成长？最近半年最大的提升是什么？', dim: 'behavioral', diff: 'easy' },
    ];

    let pool = [...technicalQuestions, ...projectQuestions, ...behavioralQuestions];

    // Filter by dimension
    if (dimension !== 'mixed') {
      pool = pool.filter((q) => q.dim === dimension);
    }

    // Filter by difficulty
    if (difficulty === 'easy') pool = pool.filter((q) => q.diff !== 'hard');
    if (difficulty === 'hard') pool = pool.filter((q) => q.diff !== 'easy');

    // Select questions
    const selected = pool.sort(() => Math.random() - 0.5).slice(0, count);

    // Add JD-specific questions if provided
    if (jdText && selected.length < count) {
      if (jdText.toLowerCase().includes('agent')) {
        selected.push({ q: '基于你对这个岗位的理解，你觉得最核心的技术挑战是什么？', dim: 'project', diff: 'hard' });
      }
    }

    return {
      success: true,
      output: JSON.stringify({
        dimension,
        difficulty,
        totalQuestions: selected.length,
        questions: selected.map((q, i) => ({
          index: i + 1,
          question: q.q,
          dimension: q.dim,
          difficulty: q.diff,
        })),
        tips: [
          '每道题用 STAR 法则组织回答（Situation → Task → Action → Result）',
          '技术题先给结论（1句话），再展开细节',
          '主动提到踩坑经验和量化结果',
        ],
      }),
    };
  },
};
