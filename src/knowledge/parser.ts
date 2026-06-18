import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { randomUUID } from 'node:crypto';
import type { KnowledgeEntry } from './types.js';

interface ParsedQuestion {
  question: string;
  noviceAnswer?: string;
  expertAnswer?: string;
  tags?: string[];
}

export function parseKnowledgeDir(dirPath: string): KnowledgeEntry[] {
  const entries: KnowledgeEntry[] = [];
  const files = walkMarkdownFiles(dirPath);

  for (const file of files) {
    const content = readFileSync(file, 'utf-8');
    const relPath = relative(dirPath, file);
    const dimension = detectDimension(relPath);
    const questions = extractQuestions(content);

    if (questions.length > 0) {
      for (const q of questions) {
        entries.push({
          id: randomUUID(),
          title: q.question.slice(0, 100),
          dimension,
          content: buildContent(q),
          sourceFile: relPath,
          question: q.question,
          expertAnswer: q.expertAnswer,
          noviceAnswer: q.noviceAnswer,
          tags: q.tags,
        });
      }
    } else {
      entries.push({
        id: randomUUID(),
        title: extractTitle(content) ?? relPath,
        dimension,
        content,
        sourceFile: relPath,
      });
    }
  }

  return entries;
}

function walkMarkdownFiles(dir: string): string[] {
  const results: string[] = [];
  const items = readdirSync(dir);

  for (const item of items) {
    const full = join(dir, item);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      results.push(...walkMarkdownFiles(full));
    } else if (item.endsWith('.md')) {
      results.push(full);
    }
  }

  return results;
}

function detectDimension(path: string): string {
  const dimensionMap: Record<string, string> = {
    '01-architecture': 'architecture',
    '02-engineering': 'engineering',
    '02-tool': 'engineering',
    '03-model': 'model',
    '03-fault': 'engineering',
    '04-rag': 'rag',
    '04-memory': 'rag',
    '05-multi-agent': 'multi-agent',
    '05-eval': 'evaluation',
    '06-evaluation': 'evaluation',
    '06-multi-agent': 'multi-agent',
    '07-full-stack': 'full-stack',
    '07-engineering': 'engineering',
    '08-prompt': 'model',
    '09-rag': 'rag',
    '10-training': 'model',
    '11-ai-code': 'full-stack',
    '12-business': 'full-stack',
    '13-project': 'architecture',
    '14-company': 'architecture',
    '15-agent': 'architecture',
    'coaching-methodology': 'coaching',
  };

  for (const [prefix, dim] of Object.entries(dimensionMap)) {
    if (path.includes(prefix)) return dim;
  }
  return 'general';
}

function extractQuestions(content: string): ParsedQuestion[] {
  const questions: ParsedQuestion[] = [];
  const qBlocks = content.split(/^#{2,3}\s*Q[：:]/m).slice(1);

  for (const block of qBlocks) {
    const lines = block.trim();
    const questionMatch = lines.match(/^(.+?)(?:\n|$)/);
    if (!questionMatch) continue;

    const question = questionMatch[1].trim();
    const noviceMatch = lines.match(/\*\*新手答\*\*[：:]?\s*"?(.+?)"?\s*(?:\n|$)/);

    const expertStartMatch = lines.match(/\*\*高手答\*\*[：:]?\s*\n/);
    let expertAnswer: string | undefined;
    if (expertStartMatch) {
      const startIdx = expertStartMatch.index! + expertStartMatch[0].length;
      const remaining = lines.slice(startIdx);
      const endMatch = remaining.match(/\n(?:\*\*差距在哪|---|\n#{2,3}\s|^\*\*追问)/m);
      expertAnswer = endMatch ? remaining.slice(0, endMatch.index).trim() : remaining.trim();
    }

    questions.push({
      question,
      noviceAnswer: noviceMatch?.[1]?.trim(),
      expertAnswer: expertAnswer?.slice(0, 2000),
    });
  }

  return questions;
}

function extractTitle(content: string): string | undefined {
  const match = content.match(/^#\s+(.+)$/m);
  return match?.[1]?.trim();
}

function buildContent(q: ParsedQuestion): string {
  let text = `问题：${q.question}\n`;
  if (q.noviceAnswer) text += `\n新手答：${q.noviceAnswer}\n`;
  if (q.expertAnswer) text += `\n高手答：${q.expertAnswer}\n`;
  return text;
}
