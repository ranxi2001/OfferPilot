import { randomUUID } from 'node:crypto';
import { DefectAnalyzer } from './defect-analyzer.js';
import type { DefectEntry, RealtimeSession, TranscriptEntry } from './types.js';

const DEFAULT_QUESTIONS = [
  '请介绍一下你在 Agent 方向的工作经历',
  '什么是 ReAct 模式？工程实现中需要注意什么？',
  '如何设计一个支持多 Provider 的 LLM 调用层？',
  'RAG 系统中 Chunk 策略有哪些选择？',
  '说一个你优化系统性能的具体案例',
];

export class RealtimeInterviewSession {
  private session: RealtimeSession;
  private analyzer = new DefectAnalyzer();
  private questions: string[];
  private questionStartTime = 0;

  constructor(questions?: string[]) {
    this.questions = questions ?? DEFAULT_QUESTIONS;
    this.session = {
      id: randomUUID(),
      state: 'idle',
      currentQuestion: null,
      questionsAsked: 0,
      totalQuestions: this.questions.length,
      transcript: [],
      defects: [],
    };
  }

  get id(): string {
    return this.session.id;
  }

  get state(): RealtimeSession['state'] {
    return this.session.state;
  }

  get currentQuestion(): string | null {
    return this.session.currentQuestion;
  }

  get progress(): { asked: number; total: number } {
    return { asked: this.session.questionsAsked, total: this.session.totalQuestions };
  }

  start(): string {
    this.session.state = 'questioning';
    return this.nextQuestion();
  }

  nextQuestion(): string {
    if (this.session.questionsAsked >= this.questions.length) {
      this.session.state = 'idle';
      return '';
    }

    const q = this.questions[this.session.questionsAsked];
    this.session.currentQuestion = q;
    this.session.state = 'questioning';
    this.questionStartTime = Date.now();

    this.session.transcript.push({
      speaker: 'interviewer',
      text: q,
      timestamp: Date.now(),
    });

    return q;
  }

  submitAnswer(answerText: string): { defects: DefectEntry[]; summary: string } {
    const elapsed = Date.now() - this.questionStartTime;

    this.session.transcript.push({
      speaker: 'candidate',
      text: answerText,
      timestamp: Date.now(),
      duration: elapsed,
    });

    this.session.state = 'analyzing';

    const defects = this.analyzer.analyze({
      question: this.session.currentQuestion ?? '',
      answer: answerText,
      elapsedMs: elapsed,
    });

    this.session.defects.push(...defects);
    this.session.questionsAsked++;
    this.session.state = 'answering';

    const critical = defects.filter((d) => d.severity === 'critical').length;
    const moderate = defects.filter((d) => d.severity === 'moderate').length;

    let summary = '';
    if (defects.length === 0) {
      summary = '✅ 这道题回答不错，没有明显缺陷';
    } else {
      summary = `发现 ${defects.length} 个问题`;
      if (critical > 0) summary += `（${critical} 个严重）`;
      summary += '：' + defects.map((d) => d.description).join('；');
    }

    return { defects, summary };
  }

  getReport(): {
    totalDefects: number;
    bySeverity: Record<string, number>;
    topIssues: string[];
    overallScore: number;
  } {
    const defects = this.session.defects;
    const bySeverity = {
      critical: defects.filter((d) => d.severity === 'critical').length,
      moderate: defects.filter((d) => d.severity === 'moderate').length,
      minor: defects.filter((d) => d.severity === 'minor').length,
    };

    // Count defect types
    const typeCounts = new Map<string, number>();
    for (const d of defects) {
      typeCounts.set(d.type, (typeCounts.get(d.type) ?? 0) + 1);
    }
    const topIssues = Array.from(typeCounts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 3)
      .map(([type, count]) => `${type} (${count}次)`);

    const maxDefects = this.session.questionsAsked * 4;
    const overallScore = Math.max(1, Math.round(10 - (defects.length / Math.max(1, maxDefects)) * 7));

    return {
      totalDefects: defects.length,
      bySeverity,
      topIssues,
      overallScore,
    };
  }

  getTranscript(): TranscriptEntry[] {
    return this.session.transcript;
  }
}
