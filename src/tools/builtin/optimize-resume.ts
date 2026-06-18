import type { ToolDefinition } from '../types.js';

const DIRECTION_STRATEGIES: Record<string, {
  narrative: string;
  dimensions: string[];
  goldenSentences: string[];
  weakenTopics: string[];
}> = {
  'agent-dev': {
    narrative: '从0到1构建 Agent 系统，擅长架构设计、Tool 开发、多步骤编排、端到端自动化',
    dimensions: ['系统架构(Agent 分层/状态流转)', 'Tool/Function 开发', '编排与调度(条件路由/并行)', '自动化率(人工→自动对比)', 'Prompt Engineering', '从0到1(独立设计)'],
    goldenSentences: [
      '"从零设计并实现了..."',
      '"Agent 自动编排..."',
      '"开发 N 个 Tool Function 覆盖..."',
      '"将人工流程 X 分钟压缩至 Y 分钟(Nx效率提升)"',
      '"端到端无人值守完成..."',
    ],
    weakenTopics: ['纯业务指标', '纯算法细节(转为Agent决策逻辑)', '纯SQL(转为Tool实现)'],
  },
  'backend': {
    narrative: '能设计高性能数据系统的后端工程师，擅长查询优化、API 设计、工具链开发',
    dimensions: ['API 设计与工程架构', '数据库与查询优化', '性能指标(QPS/延迟/数据量级)', '系统可靠性(重试/断点续传/连接池)', '工具链开发(CLI/自动化pipeline)'],
    goldenSentences: [
      '"设计 RESTful API 支撑..."',
      '"通过分片/索引/缓存优化将查询从 Xs 降至 Yms"',
      '"开发自动化 pipeline 实现..."',
      '"系统 SLA 达到 X%，QPS 支撑 Y"',
    ],
    weakenTopics: ['Agent/AI 术语', '业务流程细节', '前端实现'],
  },
  'ai-algo': {
    narrative: '模型训练与优化专家，擅长NLP/CV模型微调、数据工程、模型部署',
    dimensions: ['模型选型与微调', '量化成果(F1/准确率/AUC)', '数据工程(清洗/标注/增强)', '部署与推理优化', '实验设计与A/B测试'],
    goldenSentences: [
      '"微调后 F1-Score 从 X% 提升至 Y%"',
      '"设计数据增强策略使训练集扩充 Nx"',
      '"模型推理延迟从 Xms 优化至 Yms"',
    ],
    weakenTopics: ['工程架构细节', '前端实现', '非核心的系统设计'],
  },
  'data-analysis': {
    narrative: '从海量数据中发现洞察并驱动决策的数据分析师',
    dimensions: ['分析方法论(假设→验证→结论)', '数据规模与复杂度', '业务影响(驱动决策/节省成本)', '可视化与报告', 'SQL 与数据建模'],
    goldenSentences: [
      '"分析 X 万条数据发现..."',
      '"基于数据洞察驱动 Y 决策，带来 Z 提升"',
      '"建立数据看板支撑..."',
    ],
    weakenTopics: ['模型原理', '系统架构', '工程实现细节'],
  },
};

export const optimizeResume: ToolDefinition = {
  schema: {
    name: 'optimize_resume',
    description: '对简历的指定部分提出优化建议：方向感知包装、量化表达、STAR 法则重写、关键词补充、结构调整',
    parameters: {
      type: 'object',
      properties: {
        section: {
          type: 'string',
          enum: ['summary', 'experience', 'project', 'skills', 'education', 'full'],
          description: '要优化的简历段落类型',
        },
        content: { type: 'string', description: '该段落的原始内容' },
        targetJd: { type: 'string', description: '目标 JD（可选，用于定向优化）' },
        direction: {
          type: 'string',
          enum: ['agent-dev', 'backend', 'ai-algo', 'data-analysis', 'auto'],
          description: '目标岗位方向（auto 时根据 JD 自动识别）',
        },
        style: {
          type: 'string',
          enum: ['concise', 'detailed', 'impact-first'],
          description: '优化风格偏好',
        },
      },
      required: ['section', 'content'],
    },
  },
  riskLevel: 'low',
  async execute(input) {
    const { section, content, targetJd, direction = 'auto', style = 'impact-first' } = input as {
      section: string;
      content: string;
      targetJd?: string;
      direction?: string;
      style?: string;
    };

    const resolvedDirection = direction === 'auto' ? detectDirection(content, targetJd) : direction;

    const issues: string[] = [];
    const suggestions: string[] = [];

    // Analyze common problems
    if (!content.match(/\d+/)) {
      issues.push('缺少量化数据');
      suggestions.push('每个经历至少包含 1 个量化指标（性能 +X%、覆盖 X 用户、节省 X 小时/周）');
    }

    if (content.includes('负责') && !content.includes('实现') && !content.includes('设计')) {
      issues.push('"负责"过多，缺少动作动词');
      suggestions.push('用具体动词替换"负责"：设计、实现、主导、优化、搭建、推动');
    }

    if (section === 'experience' || section === 'project') {
      if (!content.includes('结果') && !content.includes('效果') && !content.match(/提升|降低|减少|增长/)) {
        issues.push('缺少成果描述');
        suggestions.push('每段经历用"做了什么 → 产生了什么结果"结构收尾');
      }

      const lines = content.split('\n').filter(Boolean);
      if (lines.length > 8) {
        issues.push('内容过长，重点不突出');
        suggestions.push('每段项目/经历控制在 3-5 个 bullet point，优先展示与目标岗位最相关的');
      }
    }

    if (section === 'summary') {
      if (content.length > 300) {
        issues.push('摘要过长');
        suggestions.push('个人摘要控制在 2-3 句话：「X 年 Y 领域经验 + 核心能力 + 求职意向」');
      }
      if (!content.match(/\d+\s*年/)) {
        issues.push('缺少年限');
        suggestions.push('开头用「X 年 Y 方向工程经验」定位自己');
      }
    }

    if (section === 'skills') {
      if (content.includes('熟悉') && content.split('熟悉').length > 4) {
        issues.push('「熟悉」使用过多，层次不清');
        suggestions.push('技能分三级：精通（日常主力）、熟练（项目用过）、了解（学习过），每级不超过 5 项');
      }
    }

    // STAR structure check for experience/project
    if ((section === 'experience' || section === 'project') && style === 'impact-first') {
      suggestions.push('推荐 Impact-first 格式：「[成果] 通过 [方法] 实现 [具体做法]」');
    }

    // Anti-AI-detection check
    const aiDetectionIssues = detectAIPatterns(content);
    if (aiDetectionIssues.length > 0) {
      issues.push('AI 生成痕迹过重');
      for (const issue of aiDetectionIssues) {
        suggestions.push(`反 AI 检测：${issue}`);
      }
    }

    // Direction-specific coaching
    const strategy = DIRECTION_STRATEGIES[resolvedDirection];
    if (strategy && (section === 'experience' || section === 'project')) {
      const hasDirectionKeywords = strategy.dimensions.some((d) =>
        content.toLowerCase().includes(d.split('(')[0].trim().toLowerCase()),
      );
      if (!hasDirectionKeywords) {
        issues.push(`内容缺少 ${resolvedDirection} 方向的核心叙事`);
        suggestions.push(`${resolvedDirection} 方向应突出：${strategy.dimensions.slice(0, 3).join('、')}`);
      }

      const hasWeakTopics = strategy.weakenTopics.some((topic) => {
        const keyword = topic.split('(')[0].trim();
        return content.includes(keyword);
      });
      if (hasWeakTopics) {
        suggestions.push(`建议弱化与 ${resolvedDirection} 无关的内容，或转化为该方向的语言`);
      }
    }

    // Target JD alignment
    if (targetJd) {
      const jdKeywords = targetJd.match(/[A-Z][a-zA-Z0-9.+#]+/g) ?? [];
      const contentLower = content.toLowerCase();
      const missingFromJd = jdKeywords.filter((k) => !contentLower.includes(k.toLowerCase()));
      if (missingFromJd.length > 0) {
        suggestions.push(`JD 中提到但简历缺失的关键词：${missingFromJd.slice(0, 5).join('、')}。考虑在描述中自然融入`);
      }
    }

    // Generate rewrite example with direction awareness
    let rewriteExample = '';
    if (section === 'experience' || section === 'project') {
      const firstLine = content.split('\n').find((l) => l.trim()) ?? content.slice(0, 100);
      if (strategy) {
        rewriteExample = `原文：${firstLine.trim()}\n改写示例（${resolvedDirection}方向）：${strategy.goldenSentences[0]}`;
      } else {
        rewriteExample = `原文：${firstLine.trim()}\n改写示例：「设计并实现了 [具体系统]，[量化成果]，支撑 [业务影响]」`;
      }
    }

    return {
      success: true,
      output: JSON.stringify({
        section,
        style,
        direction: resolvedDirection,
        hasTargetJd: !!targetJd,
        diagnosis: {
          score: Math.max(3, 10 - issues.length * 2),
          issues,
        },
        suggestions,
        rewriteExample: rewriteExample || undefined,
        directionCoaching: strategy
          ? {
              narrative: strategy.narrative,
              goldenSentences: strategy.goldenSentences,
              emphasize: strategy.dimensions.slice(0, 4),
              weaken: strategy.weakenTopics,
            }
          : undefined,
        principles: [
          '每个 bullet point = 动作 + 对象 + 结果',
          '数字 > 形容词（"大幅提升" → "提升 40%"）',
          '与目标岗位无关的经历可以压缩或删除',
          '同一经历在不同方向简历中用不同视角描述',
        ],
      }),
    };
  },
};

function detectDirection(content: string, jd?: string): string {
  const text = `${content} ${jd ?? ''}`.toLowerCase();

  const scores: Record<string, number> = {
    'agent-dev': 0,
    'backend': 0,
    'ai-algo': 0,
    'data-analysis': 0,
  };

  const agentKeywords = ['agent', 'tool', 'function call', 'mcp', 'langchain', 'langgraph', 'prompt', 'rag', '编排', '自动化'];
  const backendKeywords = ['api', 'rest', '数据库', 'redis', 'mysql', 'postgres', 'docker', '微服务', '高并发', 'spring'];
  const algoKeywords = ['模型', '训练', '微调', 'pytorch', 'transformer', 'bert', 'lora', 'finetune', 'f1', 'nlp'];
  const dataKeywords = ['分析', '数据', 'sql', '可视化', '报表', 'pandas', 'tableau', '指标', '洞察'];

  for (const kw of agentKeywords) if (text.includes(kw)) scores['agent-dev']++;
  for (const kw of backendKeywords) if (text.includes(kw)) scores['backend']++;
  for (const kw of algoKeywords) if (text.includes(kw)) scores['ai-algo']++;
  for (const kw of dataKeywords) if (text.includes(kw)) scores['data-analysis']++;

  const sorted = Object.entries(scores).sort((a, b) => b[1] - a[1]);
  return sorted[0][1] > 0 ? sorted[0][0] : 'agent-dev';
}

function detectAIPatterns(content: string): string[] {
  const issues: string[] = [];

  const buzzwords = ['赋能', '闭环', '抓手', '沉淀', '对齐', '拉通', '颗粒度', '底层逻辑', '打法'];
  const buzzwordHits = buzzwords.filter((w) => content.includes(w));
  if (buzzwordHits.length >= 2) {
    issues.push(`出现 ${buzzwordHits.length} 个套话词（${buzzwordHits.join('、')}），建议替换为具体描述`);
  }

  const bullets = content.split('\n').filter((l) => l.trim().startsWith('-') || /^\d+[.、]/.test(l.trim()));
  if (bullets.length >= 3) {
    const lengths = bullets.map((b) => b.length);
    const avgLen = lengths.reduce((s, l) => s + l, 0) / lengths.length;
    const variance = lengths.reduce((s, l) => s + Math.abs(l - avgLen), 0) / lengths.length;
    if (variance < 5 && bullets.length >= 4) {
      issues.push('Bullet point 长度过于对称（AI 痕迹），建议长短错落');
    }
  }

  const startsWithDesign = content.split('\n').filter((l) => l.trim().startsWith('设计并实现')).length;
  if (startsWithDesign >= 3) {
    issues.push('重复使用"设计并实现..."开头，建议变化句式（主导/推动/独立/从零）');
  }

  if (content.includes('首先') && content.includes('其次') && content.includes('最后')) {
    const nonList = !content.includes('1.') && !content.includes('第一');
    if (nonList) {
      issues.push('"首先/其次/最后"三连是典型 AI 句式，建议用更自然的过渡');
    }
  }

  if (content.length > 200 && !content.match(/具体|比如|例如|实际上|其实/)) {
    issues.push('缺少具体化词语，内容偏模板化，建议加入具体工具名/数据量/场景');
  }

  return issues;
}
