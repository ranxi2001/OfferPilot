import { NextRequest, NextResponse } from 'next/server';

export async function POST(req: NextRequest) {
  const { jd, resume } = (await req.json()) as { jd?: string; resume?: string };

  if (!jd?.trim() || !resume?.trim()) {
    return NextResponse.json({ error: 'jd and resume are required' }, { status: 400 });
  }

  const jdKeywords = extractKeywords(jd);
  const resumeKeywords = extractKeywords(resume);

  const matched = jdKeywords.filter((kw) =>
    resumeKeywords.some((rk) => rk.includes(kw) || kw.includes(rk))
  );
  const missing = jdKeywords.filter((kw) =>
    !resumeKeywords.some((rk) => rk.includes(kw) || kw.includes(rk))
  );

  const score = jdKeywords.length > 0
    ? Math.round((matched.length / jdKeywords.length) * 100)
    : 50;

  const level = detectLevel(jd);
  const focus = detectFocus(jd);
  const suggestions = generateSuggestions(missing, resume);

  return NextResponse.json({
    score,
    matched,
    missing,
    suggestions,
    level,
    focus,
  });
}

function extractKeywords(text: string): string[] {
  const techPatterns = [
    /TypeScript|JavaScript|Python|Go|Rust|Java|C\+\+/gi,
    /React|Vue|Angular|Next\.?js|Node\.?js|Express/gi,
    /Docker|Kubernetes|K8s|CI\/CD|GitHub Actions/gi,
    /PostgreSQL|MySQL|Redis|MongoDB|SQLite|Elasticsearch/gi,
    /LLM|GPT|Claude|Agent|RAG|embedding|向量/gi,
    /分布式|微服务|gRPC|REST|GraphQL|WebSocket|SSE/gi,
    /TDD|单元测试|E2E|集成测试|自动化测试/gi,
    /Webpack|Vite|ESBuild|Tailwind|shadcn/gi,
    /AWS|GCP|Azure|阿里云|腾讯云/gi,
    /机器学习|深度学习|NLP|CV|MLOps|训练|微调/gi,
    /Milvus|Qdrant|Pinecone|Faiss|向量数据库/gi,
    /Prompt|CoT|ReAct|Tool Use|Function Calling/gi,
    /架构设计|系统设计|高可用|高并发|性能优化/gi,
    /数据处理|ETL|数据管道|Spark|Flink/gi,
  ];

  const keywords = new Set<string>();
  for (const pattern of techPatterns) {
    const matches = text.match(pattern);
    if (matches) {
      matches.forEach((m) => keywords.add(m.toLowerCase().trim()));
    }
  }

  const cnKeywords = text.match(/[一-鿿]{2,6}(?:系统|架构|服务|引擎|平台|框架|协议|模型|算法|能力)/g);
  if (cnKeywords) {
    cnKeywords.forEach((kw) => keywords.add(kw));
  }

  return Array.from(keywords);
}

function detectLevel(jd: string): string {
  if (jd.match(/[5五]年以上|资深|高级|P[67]/)) return '高级工程师 (P6-P7)';
  if (jd.match(/[3三]年以上|中级|P5/)) return '中级工程师 (P5)';
  if (jd.match(/[8八]年以上|专家|架构师|P[89]/)) return '专家/架构师 (P8+)';
  return '工程师';
}

function detectFocus(jd: string): string[] {
  const areas: string[] = [];
  if (jd.match(/Agent|LLM|大模型|GPT|Claude/i)) areas.push('AI/LLM 工程');
  if (jd.match(/架构|系统设计|分布式/)) areas.push('系统架构');
  if (jd.match(/全栈|前端|后端|Web/i)) areas.push('全栈开发');
  if (jd.match(/RAG|检索|知识库|向量/)) areas.push('RAG/检索');
  if (jd.match(/数据|ETL|管道|分析/)) areas.push('数据工程');
  if (areas.length === 0) areas.push('软件工程');
  return areas;
}

function generateSuggestions(missing: string[], resume: string): string[] {
  const suggestions: string[] = [];

  if (missing.length > 5) {
    suggestions.push('JD 要求的技术栈覆盖不足，建议在项目经历中补充相关技术的使用经验');
  }

  if (missing.some((kw) => kw.match(/docker|k8s|kubernetes|容器/i))) {
    suggestions.push('补充容器化/部署相关经验，即使只是 Docker 单机部署也值得提及');
  }

  if (missing.some((kw) => kw.match(/分布式|高并发|高可用/))) {
    suggestions.push('在项目中突出系统规模（QPS、数据量、节点数），体现分布式思维');
  }

  if (missing.some((kw) => kw.match(/agent|llm|大模型|rag/i))) {
    suggestions.push('突出 AI/LLM 相关实践，包括 Prompt 工程、RAG 搭建、Agent 开发');
  }

  if (missing.some((kw) => kw.match(/测试|tdd|e2e/i))) {
    suggestions.push('补充测试实践：测试覆盖率、TDD 经验、CI 自动化');
  }

  if (!resume.match(/\d+%|\d+ms|\d+QPS|\d+万/)) {
    suggestions.push('简历中缺少量化数据，建议每个项目至少有 1-2 个数字指标');
  }

  if (suggestions.length === 0) {
    suggestions.push('匹配度良好，建议进一步强化最核心的 2-3 个技术点的深度描述');
  }

  return suggestions.slice(0, 5);
}
