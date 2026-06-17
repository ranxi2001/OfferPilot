import type { ToolDefinition, ToolContext } from '../types.js';
import type { SubAgentRuntime } from '../../sub-agent/runtime.js';

let subAgentRuntime: SubAgentRuntime | undefined;

export function setDiagnoseSubAgent(runtime: SubAgentRuntime): void {
  subAgentRuntime = runtime;
}

function ruleBasedDiagnosis(question: string, answer: string, dimension?: string) {
  const answerLen = answer.length;
  const hasStructure = answer.includes('1.') || answer.includes('- ') || answer.includes('首先');
  const hasExample = answer.includes('例如') || answer.includes('比如') || answer.includes('实际');
  const hasTechnicalDepth = answerLen > 200 && (answer.includes('因为') || answer.includes('原因'));

  let score = 4;
  if (answerLen > 100) score += 1;
  if (answerLen > 300) score += 1;
  if (hasStructure) score += 1;
  if (hasExample) score += 1;
  if (hasTechnicalDepth) score += 1;
  score = Math.min(score, 9);

  const gaps: string[] = [];
  if (!hasStructure) gaps.push('回答缺乏结构，建议用分点陈述');
  if (!hasExample) gaps.push('缺少实际案例或具体场景支撑');
  if (!hasTechnicalDepth) gaps.push('技术深度不足，缺少原理层面的解释');
  if (answerLen < 100) gaps.push('回答过于简短，信息量不够');
  if (!answer.includes('坑') && !answer.includes('注意') && !answer.includes('问题')) {
    gaps.push('缺少工程实践中的踩坑经验');
  }

  const suggestions = [
    '先用一句话给出核心定义，再展开细节',
    '主动提到"生产环境需要注意的点"展示实战经验',
    '结尾用一句话总结关键 takeaway',
  ];

  if (!hasStructure) suggestions.unshift('使用 1/2/3 分点结构组织回答');
  if (!hasExample) suggestions.unshift('加入一个具体的项目经历或技术细节');

  return {
    question,
    dimension: dimension ?? 'auto-detect',
    score: { overall: score, max: 10 },
    breakdown: {
      technicalDepth: hasTechnicalDepth ? 7 : 4,
      structure: hasStructure ? 8 : 4,
      practicalExperience: hasExample ? 7 : 3,
      completeness: Math.min(Math.floor(answerLen / 50), 8),
    },
    gaps,
    suggestions: suggestions.slice(0, 4),
    answerLength: answerLen,
    source: 'rule-based' as const,
  };
}

export const diagnoseAnswer: ToolDefinition = {
  schema: {
    name: 'diagnose_answer',
    description: '对用户的面试回答进行结构化诊断，输出评分、差距分析和改进建议',
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
