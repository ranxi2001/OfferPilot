import { NextRequest, NextResponse } from 'next/server';

export async function POST(req: NextRequest) {
  const { content } = (await req.json()) as { content?: string };

  if (!content?.trim()) {
    return NextResponse.json({ error: 'content is required' }, { status: 400 });
  }

  const sections = splitSections(content);
  const diagnosis = sections.map((section) => analyzeSection(section));

  return NextResponse.json({ diagnosis });
}

function splitSections(content: string): { title: string; text: string }[] {
  const lines = content.split('\n');
  const sections: { title: string; text: string }[] = [];
  let current: { title: string; lines: string[] } | null = null;

  for (const line of lines) {
    const headingMatch = line.match(/^#{1,3}\s+(.+)/);
    if (headingMatch) {
      if (current) sections.push({ title: current.title, text: current.lines.join('\n') });
      current = { title: headingMatch[1], lines: [] };
    } else if (current) {
      current.lines.push(line);
    } else {
      if (line.trim()) {
        current = { title: '项目经历', lines: [line] };
      }
    }
  }
  if (current) sections.push({ title: current.title, text: current.lines.join('\n') });

  if (sections.length === 0) {
    const paragraphs = content.split(/\n{2,}/);
    return paragraphs.filter((p) => p.trim()).map((p, i) => ({
      title: `段落 ${i + 1}`,
      text: p.trim(),
    }));
  }
  return sections;
}

function analyzeSection(section: { title: string; text: string }) {
  const { title, text } = section;
  const issues: string[] = [];
  const suggestions: string[] = [];
  let score = 8;

  if (text.length < 30) {
    issues.push('内容过少');
    suggestions.push('补充技术栈、成果和具体数据');
    score -= 2;
  }

  if (!text.match(/\d+/)) {
    issues.push('缺少量化数据');
    suggestions.push('加入性能指标：延迟、QPS、成功率、覆盖人数等');
    score -= 1;
  }

  if (!text.match(/[结果成果效果提升降低优化]/) && text.length > 50) {
    issues.push('未体现成果');
    suggestions.push('用 STAR 结构结尾加上 Result（成果）');
    score -= 1;
  }

  if (!text.match(/[选择|设计|架构|方案]/) && text.length > 80) {
    issues.push('未体现技术决策');
    suggestions.push('描述为什么选择该技术方案，体现判断力');
    score -= 1;
  }

  if (text.length > 60 && !text.includes('负责') && !text.includes('主导') && !text.includes('我')) {
    issues.push('未突出个人贡献');
    suggestions.push('明确个人角色："我负责..."、"我主导了..."');
    score -= 1;
  }

  const buzzwords = (text.match(/精通|熟悉|了解|掌握/g) ?? []).length;
  if (buzzwords >= 3) {
    issues.push('技能描述太泛');
    suggestions.push('用项目经验佐证技能水平，而非堆砌"精通/熟悉"');
    score -= 1;
  }

  if (issues.length === 0) {
    suggestions.push('继续保持，可适当补充更多量化数据');
  }

  return {
    section: title,
    score: Math.max(3, Math.min(10, score)),
    issues,
    suggestions,
  };
}
