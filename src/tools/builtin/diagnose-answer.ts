import type { ToolDefinition, ToolContext } from '../types.js';
import type { SubAgentRuntime } from '../../sub-agent/runtime.js';

let subAgentRuntime: SubAgentRuntime | undefined;

export function setDiagnoseSubAgent(runtime: SubAgentRuntime): void {
  subAgentRuntime = runtime;
}

const ANSWER_LEVELS = [
  { level: 'L1', name: '定义层', trait: '能背概念', signal: '回答停留在"是什么"' },
  { level: 'L2', name: '对比层', trait: '能比较优劣', signal: '有对比但无选择标准' },
  { level: 'L3', name: '决策层', trait: '能说清选择标准', signal: '有场景+理由但缺实践' },
  { level: 'L4', name: '实践层', trait: '有踩坑经验', signal: '有具体经验但未上升到体系' },
  { level: 'L5', name: '体系层', trait: '能设计完整方案', signal: '有体系观和前瞻性' },
] as const;

const LOSS_PATTERNS = [
  { pattern: '面面俱到', test: (a: string) => (a.match(/\d+[.、)）]/g)?.length ?? 0) > 6 && a.length < 400, fix: '挑最核心的 2-3 个深入讲，不要列清单' },
  { pattern: '只说是什么', test: (a: string) => !a.includes('因为') && !a.includes('所以') && !a.includes('选择'), fix: '补充"为什么选这个"和"怎么落地"' },
  { pattern: '缺少取舍', test: (a: string) => a.includes('都可以') || a.includes('都行') || (a.includes('也可以') && !a.includes('推荐')), fix: '给明确推荐 + 说清什么条件下换方案' },
  { pattern: '没有经验感', test: (a: string) => !a.includes('项目') && !a.includes('遇到') && !a.includes('实际') && !a.includes('经验'), fix: '加一个"我在 X 项目中遇到过"的真实细节' },
  { pattern: '答非所问', test: (a: string) => a.length > 500 && !a.slice(0, 100).includes('核心') && !a.slice(0, 100).includes('关键'), fix: '先用一句话直接回答核心问题，再展开' },
  { pattern: '过度简短', test: (a: string) => a.length < 80, fix: '用"具体来说..."主动展开，给出实现细节' },
] as const;

function detectAnswerLevel(answer: string): number {
  const hasDesign = answer.includes('设计') || answer.includes('架构') || answer.includes('方案');
  const hasTrdeoff = answer.includes('权衡') || answer.includes('取舍') || answer.includes('代价') || answer.includes('风险');
  const hasPractice = answer.includes('遇到') || answer.includes('踩坑') || answer.includes('生产') || answer.includes('线上');
  const hasComparison = answer.includes('对比') || answer.includes('区别') || answer.includes('优势') || answer.includes('劣势');
  const hasDecision = answer.includes('选择') || answer.includes('场景') || answer.includes('条件') || answer.includes('适合');

  if (hasDesign && hasTrdeoff && hasPractice) return 5;
  if (hasPractice && hasDecision) return 4;
  if (hasDecision || (hasComparison && answer.includes('因为'))) return 3;
  if (hasComparison) return 2;
  return 1;
}

function ruleBasedDiagnosis(question: string, answer: string, dimension?: string) {
  const answerLen = answer.length;
  const hasStructure = answer.includes('1.') || answer.includes('- ') || answer.includes('首先');
  const hasExample = answer.includes('例如') || answer.includes('比如') || answer.includes('实际') || answer.includes('项目');
  const hasTechnicalDepth = answerLen > 200 && (answer.includes('因为') || answer.includes('原因') || answer.includes('原理'));
  const hasQuantification = /\d+%|\d+[xX]|\d+倍|\d+ms|\d+万/.test(answer);
  const hasProactiveExtension = answer.includes('另外') || answer.includes('补充') || answer.includes('还需要考虑') || answer.includes('值得注意');

  const level = detectAnswerLevel(answer);

  let score = 3;
  if (answerLen > 100) score += 1;
  if (answerLen > 300) score += 0.5;
  if (hasStructure) score += 1;
  if (hasExample) score += 1;
  if (hasTechnicalDepth) score += 1;
  if (hasQuantification) score += 0.5;
  if (hasProactiveExtension) score += 0.5;
  if (level >= 4) score += 1;
  score = Math.min(Math.round(score), 9);

  const gaps: string[] = [];
  const detectedLossPatterns: string[] = [];

  for (const lp of LOSS_PATTERNS) {
    if (lp.test(answer)) {
      detectedLossPatterns.push(lp.pattern);
      gaps.push(`${lp.pattern}：${lp.fix}`);
    }
  }

  if (!hasStructure && !detectedLossPatterns.includes('过度简短')) gaps.push('回答缺乏结构，建议用分点陈述');
  if (!hasExample) gaps.push('缺少实际案例或具体场景支撑');
  if (!hasTechnicalDepth) gaps.push('技术深度不足，缺少原理层面的解释');
  if (!hasQuantification) gaps.push('缺少量化数据，建议加入具体指标');

  const currentLevel = ANSWER_LEVELS[level - 1];
  const nextLevel = level < 5 ? ANSWER_LEVELS[level] : undefined;

  const suggestions: string[] = [];
  if (nextLevel) {
    suggestions.push(`当前回答处于 ${currentLevel.level}(${currentLevel.name})，目标升级到 ${nextLevel.level}(${nextLevel.name})：需要展示${nextLevel.trait}`);
  }
  if (!hasStructure) suggestions.push('使用"第一...第二...第三..."结构组织核心论点');
  if (!hasExample) suggestions.push('加入"我在 X 项目中..."的真实经验展示实战深度');
  if (!hasTechnicalDepth) suggestions.push('补充"为什么选这个方案"的技术判断和取舍');
  if (!hasQuantification) suggestions.push('用具体数字支撑："性能提升 X%"、"覆盖 Y 个场景"');
  if (!hasProactiveExtension) suggestions.push('主动延伸"在实际中还需要考虑..."展示全面性');

  return {
    question,
    dimension: dimension ?? 'auto-detect',
    score: { overall: score, max: 10 },
    level: { current: currentLevel.level, name: currentLevel.name, trait: currentLevel.trait },
    nextLevel: nextLevel ? { target: nextLevel.level, name: nextLevel.name, requirement: nextLevel.trait } : undefined,
    breakdown: {
      technicalDepth: hasTechnicalDepth ? 7 : 4,
      structure: hasStructure ? 8 : 4,
      practicalExperience: hasExample ? 7 : 3,
      completeness: Math.min(Math.floor(answerLen / 50), 8),
      quantification: hasQuantification ? 8 : 3,
      proactivity: hasProactiveExtension ? 8 : 4,
    },
    lossPatterns: detectedLossPatterns,
    gaps,
    suggestions: suggestions.slice(0, 5),
    answerLength: answerLen,
    source: 'rule-based' as const,
  };
}

function isBehavioralQuestion(question: string): boolean {
  const behavioral = ['经历', '例子', '怎么处理', '如何解决', '遇到过', '最大的', '团队', '冲突', '失败', '困难', '挑战', '协作', 'STAR'];
  return behavioral.some((kw) => question.includes(kw));
}

function scoreBehavioralSTAR(answer: string): { starClarity: number; metrics: number; conciseness: number; reflection: number } {
  const hasSituation = answer.includes('当时') || answer.includes('背景') || answer.includes('场景');
  const hasTask = answer.includes('任务') || answer.includes('目标') || answer.includes('要求') || answer.includes('我需要');
  const hasAction = answer.includes('我做了') || answer.includes('我的做法') || answer.includes('我决定') || answer.includes('具体');
  const hasResult = /\d+%|\d+倍|结果|最终|成功|上线/.test(answer);

  const starParts = [hasSituation, hasTask, hasAction, hasResult].filter(Boolean).length;
  const starClarity = Math.min(5, starParts + (answer.length > 200 ? 1 : 0));

  const metricsCount = (answer.match(/\d+%|\d+[xX]|\d+倍|\d+万|\d+天|\d+人/g) ?? []).length;
  const metrics = Math.min(5, metricsCount + 1);

  const conciseness = answer.length < 150 ? 2 : answer.length < 400 ? 4 : answer.length < 800 ? 5 : 3;

  const hasReflection = answer.includes('反思') || answer.includes('如果重来') || answer.includes('学到') || answer.includes('改进');
  const reflection = hasReflection ? 4 : 2;

  return { starClarity, metrics, conciseness, reflection };
}

export const diagnoseAnswer: ToolDefinition = {
  schema: {
    name: 'diagnose_answer',
    description: '对用户的面试回答进行结构化诊断（技术面 L1-L5 层次 + 行为面 STAR 五维评分），输出评分、差距分析和改进建议',
    parameters: {
      type: 'object',
      properties: {
        question: { type: 'string', description: '面试题目' },
        answer: { type: 'string', description: '用户的回答内容' },
        dimension: {
          type: 'string',
          enum: ['architecture', 'engineering', 'model', 'rag', 'multi-agent', 'evaluation', 'full-stack'],
          description: '问题所属维度',
        },
      },
      required: ['question', 'answer'],
    },
  },
  riskLevel: 'low',
  async execute(input, ctx?: ToolContext) {
    const { question, answer, dimension } = input as {
      question: string;
      answer: string;
      dimension?: string;
    };

    const ruleResult = ruleBasedDiagnosis(question, answer, dimension);

    if (isBehavioralQuestion(question)) {
      const starScores = scoreBehavioralSTAR(answer);
      (ruleResult as Record<string, unknown>).behavioralRubric = {
        starClarity: { score: starScores.starClarity, max: 5 },
        metrics: { score: starScores.metrics, max: 5 },
        conciseness: { score: starScores.conciseness, max: 5 },
        reflection: { score: starScores.reflection, max: 5 },
      };
      const starAvg = (starScores.starClarity + starScores.metrics + starScores.conciseness + starScores.reflection) / 4;
      if (starAvg < 3) {
        ruleResult.suggestions.unshift('行为面建议使用 STAR 结构：Situation(背景) → Task(任务) → Action(你做了什么) → Result(量化结果)');
      }
      if (starScores.reflection <= 2) {
        ruleResult.suggestions.push('建议结尾加一句反思："如果重来，我会在 X 方面做得更好"');
      }
    }

    if (!subAgentRuntime) {
      return { success: true, output: JSON.stringify(ruleResult) };
    }

    const prompt = `请对以下面试回答进行专业诊断。

## 面试题目
${question}

## 候选人回答
${answer}

## 维度
${dimension ?? '自动识别'}

## 规则引擎初筛结果（作为参考基线）
- 初始评分: ${ruleResult.score.overall}/10
- 检测到的问题: ${ruleResult.gaps.join('; ') || '无'}

## 输出要求
请以 JSON 格式输出，包含以下字段：
- score: { overall: 1-10, max: 10 }
- breakdown: { technicalDepth: 1-10, structure: 1-10, practicalExperience: 1-10, completeness: 1-10 }
- gaps: string[] (具体差距点)
- suggestions: string[] (最多4条改进建议)
- highlights: string[] (回答中的亮点，如果有)

只输出 JSON，不要有其他文字。`;

    const result = await subAgentRuntime.dispatch({
      agentId: 'diagnostician',
      input: prompt,
      parentSessionId: ctx?.sessionId ?? 'system',
    });

    if (!result.success) {
      return { success: true, output: JSON.stringify(ruleResult) };
    }

    try {
      const jsonMatch = result.output.match(/\{[\s\S]*\}/);
      if (jsonMatch) {
        const llmResult = JSON.parse(jsonMatch[0]);
        return {
          success: true,
          output: JSON.stringify({
            question,
            dimension: dimension ?? 'auto-detect',
            ...llmResult,
            answerLength: answer.length,
            source: 'llm-diagnosed',
          }),
        };
      }
    } catch {
      // LLM output parse failed, fall back to rule-based
    }

    return { success: true, output: JSON.stringify(ruleResult) };
  },
};
