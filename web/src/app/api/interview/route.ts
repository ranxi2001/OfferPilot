import { NextRequest, NextResponse } from 'next/server';

interface InterviewSession {
  id: string;
  questions: string[];
  currentIndex: number;
  state: 'idle' | 'questioning' | 'answering';
  transcript: { speaker: string; text: string; timestamp: number }[];
  defects: DefectEntry[];
  questionStartTime: number;
}

interface DefectEntry {
  id: string;
  type: string;
  severity: 'minor' | 'moderate' | 'critical';
  description: string;
  suggestion: string;
}

const DEFAULT_QUESTIONS = [
  '请介绍一下你在 Agent 方向的工作经历',
  '什么是 ReAct 模式？工程实现中需要注意什么？',
  '如何设计一个支持多 Provider 的 LLM 调用层？',
  'RAG 系统中 Chunk 策略有哪些选择？各自适合什么场景？',
  '说一个你优化系统性能的具体案例',
];

const sessions = new Map<string, InterviewSession>();

export async function POST(req: NextRequest) {
  const body = await req.json();
  const { action, sessionId, answer, questions } = body as {
    action: 'start' | 'answer' | 'next' | 'report';
    sessionId?: string;
    answer?: string;
    questions?: string[];
  };

  if (action === 'start') {
    const id = crypto.randomUUID();
    const qs = questions?.length ? questions : DEFAULT_QUESTIONS;
    const session: InterviewSession = {
      id,
      questions: qs,
      currentIndex: 0,
      state: 'questioning',
      transcript: [],
      defects: [],
      questionStartTime: Date.now(),
    };

    const firstQuestion = qs[0];
    session.transcript.push({ speaker: 'interviewer', text: firstQuestion, timestamp: Date.now() });
    sessions.set(id, session);

    return NextResponse.json({
      sessionId: id,
      question: firstQuestion,
      progress: { current: 1, total: qs.length },
    });
  }

  const session = sessionId ? sessions.get(sessionId) : undefined;
  if (!session) {
    return NextResponse.json({ error: 'Session not found' }, { status: 404 });
  }

  if (action === 'answer') {
    if (!answer) {
      return NextResponse.json({ error: 'answer is required' }, { status: 400 });
    }

    const elapsed = Date.now() - session.questionStartTime;
    session.transcript.push({ speaker: 'candidate', text: answer, timestamp: Date.now() });

    const defects = analyzeDefects(session.questions[session.currentIndex], answer, elapsed);
    session.defects.push(...defects);
    session.currentIndex++;

    const hasNext = session.currentIndex < session.questions.length;
    session.state = hasNext ? 'answering' : 'idle';

    return NextResponse.json({
      defects,
      summary: buildSummary(defects),
      hasNext,
      progress: { current: session.currentIndex, total: session.questions.length },
    });
  }

  if (action === 'next') {
    if (session.currentIndex >= session.questions.length) {
      return NextResponse.json({ error: 'No more questions', done: true });
    }

    const question = session.questions[session.currentIndex];
    session.state = 'questioning';
    session.questionStartTime = Date.now();
    session.transcript.push({ speaker: 'interviewer', text: question, timestamp: Date.now() });

    return NextResponse.json({
      question,
      progress: { current: session.currentIndex + 1, total: session.questions.length },
    });
  }

  if (action === 'report') {
    const defects = session.defects;
    const bySeverity = {
      critical: defects.filter((d) => d.severity === 'critical').length,
      moderate: defects.filter((d) => d.severity === 'moderate').length,
      minor: defects.filter((d) => d.severity === 'minor').length,
    };

    const typeCounts = new Map<string, number>();
    for (const d of defects) {
      typeCounts.set(d.type, (typeCounts.get(d.type) ?? 0) + 1);
    }
    const topIssues = Array.from(typeCounts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 3)
      .map(([type, count]) => ({ type, count }));

    const maxDefects = session.currentIndex * 4;
    const overallScore = Math.max(1, Math.round(10 - (defects.length / Math.max(1, maxDefects)) * 7));

    sessions.delete(session.id);

    return NextResponse.json({
      totalQuestions: session.currentIndex,
      totalDefects: defects.length,
      bySeverity,
      topIssues,
      overallScore,
      transcript: session.transcript,
    });
  }

  return NextResponse.json({ error: 'Invalid action' }, { status: 400 });
}

function analyzeDefects(question: string, answer: string, elapsedMs: number): DefectEntry[] {
  const defects: DefectEntry[] = [];
  let idCounter = 0;
  const mkId = () => `d${Date.now()}-${idCounter++}`;

  if (answer.length < 50) {
    defects.push({ id: mkId(), type: 'too_short', severity: 'critical', description: '回答过于简短，缺乏有效信息', suggestion: '至少展开 2-3 个要点，每个要点一句话' });
  }

  if (answer.length > 100 && !answer.match(/[1-9一二三四五六七八九十][.、)）]/) && !answer.includes('首先') && !answer.includes('其次')) {
    defects.push({ id: mkId(), type: 'no_structure', severity: 'moderate', description: '回答缺乏结构，一段到底', suggestion: '用"第一…第二…第三…"或"首先…其次…最后…"组织' });
  }

  if (answer.length > 80 && !answer.includes('例如') && !answer.includes('比如') && !answer.includes('实际') && !answer.includes('项目')) {
    defects.push({ id: mkId(), type: 'missing_example', severity: 'moderate', description: '缺少具体案例支撑', suggestion: '加一句"比如在我之前的项目中…"增强说服力' });
  }

  const vagueWords = (answer.match(/可能|大概|好像|一些|某些|差不多/g) ?? []).length;
  if (vagueWords >= 3) {
    defects.push({ id: mkId(), type: 'too_vague', severity: 'moderate', description: `模糊表述过多（${vagueWords} 处）`, suggestion: '用具体数字和明确说法替换"大概""可能"' });
  }

  if (answer.length > 150 && !answer.includes('因为') && !answer.includes('原因') && !answer.includes('本质')) {
    defects.push({ id: mkId(), type: 'no_depth', severity: 'minor', description: '停留在表面描述，缺少原理分析', suggestion: '补充一句"之所以这样做是因为…"展示深度理解' });
  }

  const fillers = (answer.match(/那个|就是说|嗯|额|然后就|对吧/g) ?? []).length;
  if (fillers >= 3) {
    defects.push({ id: mkId(), type: 'filler_words', severity: 'minor', description: `口头禅过多（${fillers} 处）`, suggestion: '放慢语速，用短暂停顿替代"嗯""那个"' });
  }

  if (elapsedMs > 15000 && answer.length < 100) {
    defects.push({ id: mkId(), type: 'hesitation', severity: 'minor', description: '思考时间过长', suggestion: '先说"这个问题我从X角度来回答"争取思考时间' });
  }

  return defects;
}

function buildSummary(defects: DefectEntry[]): string {
  if (defects.length === 0) return '这道题回答不错，没有明显缺陷';
  const critical = defects.filter((d) => d.severity === 'critical').length;
  let summary = `发现 ${defects.length} 个问题`;
  if (critical > 0) summary += `（${critical} 个严重）`;
  return summary;
}
