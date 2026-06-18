import type { ToolDefinition } from '../types.js';

const ATS_WEIGHTS = {
  keywordMatch: 0.30,
  experienceRelevance: 0.15,
  formatCompliance: 0.15,
  actionVerbs: 0.10,
  titleMatch: 0.10,
  quantification: 0.10,
  readability: 0.10,
} as const;

export const matchResumeJd: ToolDefinition = {
  schema: {
    name: 'match_resume_jd',
    description: '将简历与 JD 进行 ATS 加权匹配度分析（7 维度评分），找出匹配项、差距项和方向感知包装建议',
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

    // Extract keywords from JD with weight classification
    const jdKeywords = extractSignificantWords(jdText);
    const hardSkills = jdKeywords.filter((k) => /^[A-Z]/.test(k));
    const softSkills = jdKeywords.filter((k) => !/^[A-Z]/.test(k));

    const matched: string[] = [];
    const missing: string[] = [];

    for (const kw of jdKeywords) {
      if (resumeLower.includes(kw.toLowerCase())) {
        matched.push(kw);
      } else {
        missing.push(kw);
      }
    }

    // ATS 7-Dimension Scoring
    const keywordScore = jdKeywords.length > 0
      ? (matched.length / jdKeywords.length) * 100
      : 50;

    const experienceScore = scoreExperienceRelevance(resumeText, jdText);
    const formatScore = scoreFormatCompliance(resumeText);
    const actionVerbScore = scoreActionVerbs(resumeText);
    const titleScore = scoreTitleMatch(resumeText, jdText);
    const quantScore = scoreQuantification(resumeText);
    const readabilityScore = scoreReadability(resumeText);

    const weightedTotal = Math.round(
      keywordScore * ATS_WEIGHTS.keywordMatch +
      experienceScore * ATS_WEIGHTS.experienceRelevance +
      formatScore * ATS_WEIGHTS.formatCompliance +
      actionVerbScore * ATS_WEIGHTS.actionVerbs +
      titleScore * ATS_WEIGHTS.titleMatch +
      quantScore * ATS_WEIGHTS.quantification +
      readabilityScore * ATS_WEIGHTS.readability,
    );

    const grade = weightedTotal >= 90 ? 'A' : weightedTotal >= 80 ? 'B' : weightedTotal >= 70 ? 'C' : weightedTotal >= 60 ? 'D' : 'F';

    // Detect resume strengths not in JD
    const resumeKeywords = extractSignificantWords(resumeText);
    const uniqueStrengths = resumeKeywords
      .filter((k) => !jdLower.includes(k.toLowerCase()))
      .slice(0, 5);

    // Direction-aware packaging suggestions
    const suggestions: string[] = [];

    if (missing.length > 0) {
      const hardMissing = missing.filter((k) => /^[A-Z]/.test(k)).slice(0, 3);
      const softMissing = missing.filter((k) => !/^[A-Z]/.test(k)).slice(0, 2);
      if (hardMissing.length > 0) {
        suggestions.push(`硬技能缺失（权重 2x）：${hardMissing.join('、')}。即使经验不深，也要在项目描述中体现`);
      }
      if (softMissing.length > 0) {
        suggestions.push(`方向性关键词缺失：${softMissing.join('、')}。建议在摘要或项目描述中自然融入`);
      }
    }

    if (quantScore < 60) {
      suggestions.push('量化不足：每段经历至少 1 个数字（"提升 X%"、"覆盖 X 用户"、"节省 X 小时/周"）');
    }

    if (actionVerbScore < 60) {
      suggestions.push('动词偏弱：用"设计、实现、主导、优化、搭建"替代"负责、参与、协助"');
    }

    if (formatScore < 70) {
      suggestions.push('格式不利于 ATS：使用标准分节标题，避免纯表格/图片，确保文字可解析');
    }

    if (weightedTotal < 50) {
      suggestions.push('综合匹配度偏低，建议针对这个 JD 定制简历：只保留相关经历，用目标方向语言重写描述');
    }

    if (uniqueStrengths.length > 0) {
      suggestions.push(`差异化优势（${uniqueStrengths.slice(0, 3).join('、')}）：面试时主动展开，这是你区别于其他候选人的亮点`);
    }

    return {
      success: true,
      output: JSON.stringify({
        atsScore: weightedTotal,
        grade,
        focusArea,
        dimensionScores: {
          keywordMatch: { score: Math.round(keywordScore), weight: '30%' },
          experienceRelevance: { score: experienceScore, weight: '15%' },
          formatCompliance: { score: formatScore, weight: '15%' },
          actionVerbs: { score: actionVerbScore, weight: '10%' },
          titleMatch: { score: titleScore, weight: '10%' },
          quantification: { score: quantScore, weight: '10%' },
          readability: { score: readabilityScore, weight: '10%' },
        },
        matched: matched.slice(0, 10),
        missing: missing.slice(0, 10),
        uniqueStrengths,
        suggestions,
        verdict: weightedTotal >= 80
          ? 'ATS 通过率高，面试准备是关键'
          : weightedTotal >= 60
            ? '基本能过 ATS，针对缺失维度微调后投递'
            : '高风险被 ATS 过滤，建议大幅调整简历定位',
      }),
    };
  },
};

function scoreExperienceRelevance(resume: string, jd: string): number {
  const jdYears = jd.match(/(\d+)\s*[年+]/);
  const resumeYears = resume.match(/(\d+)\s*年/);
  let score = 60;
  if (jdYears && resumeYears) {
    const required = parseInt(jdYears[1]);
    const have = parseInt(resumeYears[1]);
    if (have >= required) score = 90;
    else if (have >= required - 1) score = 70;
    else score = 40;
  }
  const jdDomainWords = jd.match(/[一-鿿]{2,4}(?:经验|背景|方向)/g) ?? [];
  const resumeLower = resume.toLowerCase();
  const domainHits = jdDomainWords.filter((w) => resumeLower.includes(w.replace(/经验|背景|方向/, ''))).length;
  score += Math.min(domainHits * 5, 20);
  return Math.min(score, 100);
}

function scoreFormatCompliance(resume: string): number {
  let score = 70;
  const hasStandardSections = ['教育', '经历', '项目', '技能'].filter((s) => resume.includes(s)).length;
  score += hasStandardSections * 5;
  if (resume.split('\n').length > 5) score += 10;
  if (resume.length > 300) score += 5;
  return Math.min(score, 100);
}

function scoreActionVerbs(resume: string): number {
  const strong = ['设计', '实现', '主导', '优化', '搭建', '推动', '构建', '开发', '部署', '解决'];
  const weak = ['负责', '参与', '协助', '了解', '熟悉'];
  const strongCount = strong.filter((v) => resume.includes(v)).length;
  const weakCount = weak.filter((v) => resume.includes(v)).length;
  const ratio = strongCount / Math.max(strongCount + weakCount, 1);
  return Math.round(ratio * 100);
}

function scoreTitleMatch(resume: string, jd: string): number {
  const jdTitle = jd.match(/(?:岗位|职位|角色)[：:]\s*(.+?)(?:\n|$)/)?.[1] ?? '';
  const titleWords = jdTitle.split(/\s+|\//).filter((w) => w.length >= 2);
  if (titleWords.length === 0) return 60;
  const hits = titleWords.filter((w) => resume.includes(w)).length;
  return Math.round((hits / titleWords.length) * 100);
}

function scoreQuantification(resume: string): number {
  const quantPatterns = [/\d+%/, /\d+[xX]/, /\d+倍/, /\d+万/, /\d+[kK]/, /\d+ms/, /\d+\s*秒/, /\d+\s*用户/];
  const hits = quantPatterns.filter((p) => p.test(resume)).length;
  return Math.min(hits * 15 + 20, 100);
}

function scoreReadability(resume: string): number {
  const lines = resume.split('\n').filter((l) => l.trim());
  const avgLineLen = lines.reduce((s, l) => s + l.length, 0) / Math.max(lines.length, 1);
  let score = 70;
  if (avgLineLen > 20 && avgLineLen < 80) score += 15;
  if (lines.length >= 10 && lines.length <= 60) score += 15;
  return Math.min(score, 100);
}

function extractSignificantWords(text: string): string[] {
  const techPattern = /[A-Z][a-zA-Z0-9.+#]*(?:\s*[A-Z][a-zA-Z0-9.+#]*)*/g;
  const cnPattern = /[一-鿿]{2,6}(?:能力|经验|设计|开发|架构|优化|系统|平台|框架|引擎|模型)/g;

  const techMatches = text.match(techPattern) ?? [];
  const cnMatches = text.match(cnPattern) ?? [];

  const keywords = [...new Set([...techMatches, ...cnMatches])];
  return keywords.filter((k) => k.length >= 2 && k.length <= 30);
}
