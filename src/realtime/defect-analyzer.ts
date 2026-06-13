import type { DefectEntry, DefectType } from './types.js';
import { randomUUID } from 'node:crypto';

interface AnalysisInput {
  question: string;
  answer: string;
  elapsedMs: number;
}

export class DefectAnalyzer {
  analyze(input: AnalysisInput): DefectEntry[] {
    const { question, answer, elapsedMs } = input;
    const defects: DefectEntry[] = [];
    const now = Date.now();

    // Too short
    if (answer.length < 50) {
      defects.push(this.create('too_short', 'critical', '回答过于简短，缺乏有效信息', '至少展开 2-3 个要点，每个要点一句话', now));
    }

    // No structure
    if (answer.length > 100 && !answer.match(/[1-9一二三四五六七八九十][.、)）]/) && !answer.includes('首先') && !answer.includes('其次')) {
      defects.push(this.create('no_structure', 'moderate', '回答缺乏结构，一段到底', '用"第一…第二…第三…"或"首先…其次…最后…"组织', now));
    }

    // Missing example
    if (answer.length > 80 && !answer.includes('例如') && !answer.includes('比如') && !answer.includes('实际') && !answer.includes('项目') && !answer.includes('之前')) {
      defects.push(this.create('missing_example', 'moderate', '缺少具体案例支撑', '加一句"比如在我之前的项目中…"增强说服力', now));
    }

    // Too vague
    const vagueWords = (answer.match(/可能|大概|好像|一些|某些|差不多/g) ?? []).length;
    if (vagueWords >= 3) {
      defects.push(this.create('too_vague', 'moderate', `模糊表述过多（${vagueWords} 处）`, '用具体数字和明确说法替换"大概""可能"', now));
    }

    // No depth
    if (answer.length > 150 && !answer.includes('因为') && !answer.includes('原因') && !answer.includes('本质') && !answer.includes('底层')) {
      defects.push(this.create('no_depth', 'minor', '停留在表面描述，缺少原理分析', '补充一句"之所以这样做是因为…"展示深度理解', now));
    }

    // Filler words
    const fillers = (answer.match(/那个|就是说|嗯|额|然后就|对吧/g) ?? []).length;
    if (fillers >= 3) {
      defects.push(this.create('filler_words', 'minor', `口头禅/填充词过多（${fillers} 处）`, '放慢语速，用短暂停顿替代"嗯""那个"', now));
    }

    // Hesitation (took too long to start answering)
    if (elapsedMs > 15000 && answer.length < 100) {
      defects.push(this.create('hesitation', 'minor', '思考时间过长，可能让面试官觉得不熟悉', '先说一句"这个问题我从X角度来回答"争取思考时间', now));
    }

    // Off topic check (very basic)
    const questionKeywords = question.match(/[一-鿿]{2,4}/g) ?? [];
    const answerLower = answer.toLowerCase();
    const relevantHits = questionKeywords.filter((kw) => answerLower.includes(kw)).length;
    if (questionKeywords.length > 3 && relevantHits < questionKeywords.length * 0.2) {
      defects.push(this.create('off_topic', 'critical', '回答可能偏题，与问题关键词重合度低', '先复述问题核心，再展开回答，确保不跑偏', now));
    }

    return defects;
  }

  private create(type: DefectType, severity: DefectEntry['severity'], description: string, suggestion: string, timestamp: number): DefectEntry {
    return { id: randomUUID().slice(0, 8), type, severity, description, timestamp, suggestion };
  }
}
