import type { ToolDefinition } from '../types.js';

export const optimizeResume: ToolDefinition = {
  schema: {
    name: 'optimize_resume',
    description: '对简历的指定部分提出优化建议：量化表达、STAR 法则重写、关键词补充、结构调整',
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
    const { section, content, targetJd, style = 'impact-first' } = input as {
      section: string;
      content: string;
      targetJd?: string;
      style?: string;
    };

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

    // Target JD alignment
    if (targetJd) {
      const jdKeywords = targetJd.match(/[A-Z][a-zA-Z0-9.+#]+/g) ?? [];
      const contentLower = content.toLowerCase();
      const missingFromJd = jdKeywords.filter((k) => !contentLower.includes(k.toLowerCase()));
      if (missingFromJd.length > 0) {
        suggestions.push(`JD 中提到但简历缺失的关键词：${missingFromJd.slice(0, 5).join('、')}。考虑在描述中自然融入`);
      }
    }

    // Generate rewrite example
    let rewriteExample = '';
    if (section === 'experience' || section === 'project') {
      const firstLine = content.split('\n').find((l) => l.trim()) ?? content.slice(0, 100);
      rewriteExample = `原文：${firstLine.trim()}\n改写示例：「设计并实现了 [具体系统]，[量化成果]，支撑 [业务影响]」`;
    }

    return {
      success: true,
      output: JSON.stringify({
        section,
        style,
        hasTargetJd: !!targetJd,
        diagnosis: {
          score: Math.max(3, 10 - issues.length * 2),
          issues,
        },
        suggestions,
        rewriteExample: rewriteExample || undefined,
        principles: [
          '每个 bullet point = 动作 + 对象 + 结果',
          '数字 > 形容词（"大幅提升" → "提升 40%"）',
          '与目标岗位无关的经历可以压缩或删除',
        ],
      }),
    };
  },
};
