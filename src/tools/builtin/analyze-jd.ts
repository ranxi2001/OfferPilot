import type { ToolDefinition } from '../types.js';

const TECH_KEYWORDS = [
  'Python', 'Java', 'Go', 'TypeScript', 'Rust', 'C++',
  'LLM', 'Agent', 'RAG', 'NLP', 'ML', 'Deep Learning',
  'LangChain', 'LangGraph', 'PyTorch', 'TensorFlow',
  'Kubernetes', 'Docker', 'AWS', 'GCP', 'Azure',
  'React', 'Next.js', 'Node.js', 'FastAPI', 'Spring',
  'PostgreSQL', 'Redis', 'MongoDB', 'Elasticsearch',
  'CI/CD', 'Microservices', 'System Design',
  'Prompt Engineering', 'Fine-tuning', 'RLHF', 'Embedding',
  'Vector Database', 'Milvus', 'Pinecone', 'Weaviate',
];

const SOFT_SKILLS = [
  '沟通', '协作', '领导力', '自驱', '抗压',
  '跨团队', '快速学习', '创新', '解决问题', '项目管理',
];

export const analyzeJd: ToolDefinition = {
  schema: {
    name: 'analyze_jd',
    description: '解析职位描述（JD），提取硬性要求、技术栈、加分项、团队信息和面试准备重点',
    parameters: {
      type: 'object',
      properties: {
        jdText: { type: 'string', description: '职位描述全文' },
        targetLevel: {
          type: 'string',
          enum: ['junior', 'mid', 'senior', 'staff', 'principal'],
          description: '目标职级（可选，辅助分析难度）',
        },
      },
      required: ['jdText'],
    },
  },
  riskLevel: 'low',
  async execute(input) {
    const { jdText, targetLevel } = input as { jdText: string; targetLevel?: string };
    const lower = jdText.toLowerCase();

    // Extract tech requirements
    const techRequired: string[] = [];
    const techNiceToHave: string[] = [];
    for (const kw of TECH_KEYWORDS) {
      if (lower.includes(kw.toLowerCase())) {
        if (jdText.includes('必须') || jdText.includes('required') || jdText.includes('熟练')) {
          techRequired.push(kw);
        } else {
          techNiceToHave.push(kw);
        }
      }
    }
    // If no clear distinction, split 60/40
    if (techRequired.length === 0 && techNiceToHave.length > 0) {
      const split = Math.ceil(techNiceToHave.length * 0.6);
      techRequired.push(...techNiceToHave.splice(0, split));
    }

    // Extract soft skills
    const softSkills = SOFT_SKILLS.filter((s) => jdText.includes(s));

    // Detect experience requirements
    const expMatch = jdText.match(/(\d+)[+\-~～至]?\s*年/);
    const yearsRequired = expMatch ? parseInt(expMatch[1]) : undefined;

    // Detect education
    const eduKeywords = ['本科', '硕士', '博士', 'Bachelor', 'Master', 'PhD'];
    const education = eduKeywords.find((e) => jdText.includes(e)) ?? '未明确';

    // Detect team/product info
    const teamHints: string[] = [];
    if (lower.includes('创业') || lower.includes('startup')) teamHints.push('创业团队');
    if (lower.includes('大模型') || lower.includes('llm')) teamHints.push('大模型方向');
    if (lower.includes('agent')) teamHints.push('Agent 产品');
    if (lower.includes('平台') || lower.includes('platform')) teamHints.push('平台型产品');
    if (lower.includes('toB') || lower.includes('企业')) teamHints.push('To B');
    if (lower.includes('toC') || lower.includes('用户')) teamHints.push('To C');

    // Interview prep suggestions
    const prepFocus: string[] = [];
    if (techRequired.some((t) => ['LLM', 'Agent', 'RAG', 'Prompt Engineering'].includes(t))) {
      prepFocus.push('Agent 架构设计 & ReAct 循环实现');
      prepFocus.push('RAG 全链路（Chunk → Embedding → Retrieval → Rerank）');
    }
    if (techRequired.some((t) => ['System Design', 'Microservices', 'Kubernetes'].includes(t))) {
      prepFocus.push('系统设计（高并发、分布式）');
    }
    if (techRequired.some((t) => ['Python', 'Go', 'Java'].includes(t))) {
      prepFocus.push('语言基础 & 算法题');
    }
    if (softSkills.length > 0) {
      prepFocus.push('行为面试（STAR 法则准备 3-5 个案例）');
    }
    if (prepFocus.length === 0) {
      prepFocus.push('技术深度 + 项目经验复盘');
    }

    const level = targetLevel ?? (yearsRequired && yearsRequired >= 5 ? 'senior' : yearsRequired && yearsRequired >= 3 ? 'mid' : 'junior');

    return {
      success: true,
      output: JSON.stringify({
        summary: {
          level,
          yearsRequired,
          education,
          teamHints,
        },
        techStack: {
          required: techRequired,
          niceToHave: techNiceToHave,
        },
        softSkills,
        interviewPrep: prepFocus,
        keyInsights: [
          techRequired.length > 5 ? '技术栈要求广，建议聚焦 top 3 核心技能深度准备' : '技术栈聚焦，建议把核心技能吃透',
          yearsRequired && yearsRequired >= 5 ? '高年资岗位，项目深度和架构能力是重点' : '注意基础扎实度，算法和编码要熟练',
          teamHints.includes('Agent 产品') ? '明确的 Agent 方向，OfferPilot 知识库的面试题高度相关' : '',
        ].filter(Boolean),
      }),
    };
  },
};
