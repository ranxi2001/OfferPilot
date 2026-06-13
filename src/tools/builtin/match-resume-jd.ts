import type { ToolDefinition } from '../types.js';

export const matchResumeJd: ToolDefinition = {
  schema: {
    name: 'match_resume_jd',
    description: '将简历与 JD 进行匹配度分析，找出匹配项、差距项和包装建议',
    parameters: {
      type: 'object',
      properties: {
        resumeText: { type: 'string', description: '简历内容（纯文本）' },
        jdText: { type: 'string', description: '职位描述全文' },
        focusArea: {
          type: 'string',
          enum: ['tech', 'experience', 'overall'],
          description: '分析侧重（默认 overall）',
        },
      },
      required: ['resumeText', 'jdText'],
    },
  },
  riskLevel: 'low',
  async execute(input) {
    const { resumeText, jdText, focusArea = 'overall' } = input as {
      resumeText: string;
      jdText: string;
      focusArea?: string;
    };

    const resumeLower = resumeText.toLowerCase();
    const jdLower = jdText.toLowerCase();

    // Extract keywords from JD
    const jdKeywords = extractSignificantWords(jdText);
    const matched: string[] = [];
    const missing: string[] = [];

    for (const kw of jdKeywords) {
      if (resumeLower.includes(kw.toLowerCase())) {
        matched.push(kw);
      } else {
        missing.push(kw);
      }
    }

    const matchRate = jdKeywords.length > 0
      ? Math.round((matched.length / jdKeywords.length) * 100)
      : 50;

    // Detect resume strengths not in JD (potential differentiation)
    const resumeKeywords = extractSignificantWords(resumeText);
    const uniqueStrengths = resumeKeywords
      .filter((k) => !jdLower.includes(k.toLowerCase()))
      .slice(0, 5);

    // Generate packaging suggestions
    const suggestions: string[] = [];

    if (missing.length > 0) {
      const topMissing = missing.slice(0, 3);
      suggestions.push(`简历中缺少 JD 关键词：${topMissing.join('、')}。即使经验不深，也建议在项目描述中体现相关实践`);
    }

    if (matchRate < 50) {
      suggestions.push('匹配度偏低，建议针对这个 JD 定制一版简历，突出相关经验');
    }

    if (resumeText.length < 500) {
      suggestions.push('简历内容偏短，建议补充项目细节：技术选型理由、量化成果、难点突破');
    }

    if (!resumeText.includes('数据') && !resumeText.includes('%') && !resumeText.match(/\d+[万kK]/)) {
      suggestions.push('缺少量化数据，建议每个项目加 1-2 个数字（性能提升 X%、服务 X 用户等）');
    }

    if (uniqueStrengths.length > 0) {
      suggestions.push(`你的独特优势（${uniqueStrengths.slice(0, 3).join('、')}）可以作为差异化竞争力，在面试中主动展开`);
    }

    return {
      success: true,
      output: JSON.stringify({
        matchRate: `${matchRate}%`,
        focusArea,
        matched: matched.slice(0, 10),
        missing: missing.slice(0, 10),
        uniqueStrengths,
        suggestions,
        verdict: matchRate >= 70
          ? '匹配度高，按当前简历投递问题不大，面试准备是关键'
          : matchRate >= 50
            ? '基本匹配，建议针对缺失项做简历微调后投递'
            : '匹配度偏低，需要大幅调整简历定位或考虑更匹配的岗位',
      }),
    };
  },
};

function extractSignificantWords(text: string): string[] {
  const techPattern = /[A-Z][a-zA-Z0-9.+#]*(?:\s*[A-Z][a-zA-Z0-9.+#]*)*/g;
  const cnPattern = /[一-鿿]{2,6}(?:能力|经验|设计|开发|架构|优化|系统|平台|框架|引擎|模型)/g;

  const techMatches = text.match(techPattern) ?? [];
  const cnMatches = text.match(cnPattern) ?? [];

  const keywords = [...new Set([...techMatches, ...cnMatches])];
  return keywords.filter((k) => k.length >= 2 && k.length <= 30);
}
