import { NextRequest, NextResponse } from 'next/server';

interface DiagnosisRecord {
  id: string;
  timestamp: number;
  dimension: string;
  score: number;
  question: string;
}

interface SM2State {
  dimension: string;
  easeFactor: number;
  interval: number;
  repetitions: number;
  nextReview: number;
}

const records: DiagnosisRecord[] = [];
const sm2States: Map<string, SM2State> = new Map();

function sm2Update(state: SM2State, score: number): SM2State {
  const quality = Math.min(5, Math.max(0, Math.round((score / 10) * 5)));
  let { easeFactor, interval, repetitions } = state;

  if (quality >= 3) {
    if (repetitions === 0) interval = 1;
    else if (repetitions === 1) interval = 3;
    else interval = Math.round(interval * easeFactor);
    repetitions++;
  } else {
    repetitions = 0;
    interval = 1;
  }

  easeFactor = Math.max(1.3, easeFactor + (0.1 - (5 - quality) * (0.08 + (5 - quality) * 0.02)));
  const nextReview = Date.now() + interval * 24 * 60 * 60 * 1000;

  return { ...state, easeFactor, interval, repetitions, nextReview };
}

export async function GET() {
  const dimensions = ['architecture', 'engineering', 'model', 'rag', 'multi-agent', 'evaluation', 'full-stack'];

  const dimensionScores = dimensions.map((dim) => {
    const dimRecords = records.filter((r) => r.dimension === dim);
    const avg = dimRecords.length > 0
      ? Math.round(dimRecords.reduce((sum, r) => sum + r.score, 0) / dimRecords.length)
      : 0;
    return { dimension: dim, score: avg, count: dimRecords.length };
  });

  const totalAnswered = records.length;
  const avgScore = records.length > 0
    ? Math.round(records.reduce((sum, r) => sum + r.score, 0) / records.length)
    : 0;

  const weakDimensions = dimensionScores
    .filter((d) => d.count > 0 && d.score < 6)
    .map((d) => d.dimension);

  const recent = records.slice(-10).reverse();

  const reviewPriority = dimensions
    .map((dim) => {
      const state = sm2States.get(dim);
      if (!state) return { dimension: dim, urgency: 0, daysUntilReview: null };
      const daysUntil = Math.round((state.nextReview - Date.now()) / (24 * 60 * 60 * 1000));
      return { dimension: dim, urgency: daysUntil <= 0 ? 10 : Math.max(0, 5 - daysUntil), daysUntilReview: daysUntil };
    })
    .filter((r) => r.urgency > 0 || r.daysUntilReview !== null)
    .sort((a, b) => b.urgency - a.urgency);

  return NextResponse.json({
    dimensionScores,
    totalAnswered,
    avgScore,
    weakDimensions,
    recent,
    reviewPriority,
  });
}

export async function POST(req: NextRequest) {
  const body = (await req.json()) as {
    dimension?: string;
    score?: number;
    question?: string;
  };

  if (!body.dimension || body.score === undefined) {
    return NextResponse.json({ error: 'dimension and score are required' }, { status: 400 });
  }

  const record: DiagnosisRecord = {
    id: crypto.randomUUID(),
    timestamp: Date.now(),
    dimension: body.dimension,
    score: Math.max(1, Math.min(10, body.score)),
    question: body.question ?? '',
  };

  records.push(record);

  const existing = sm2States.get(body.dimension) ?? {
    dimension: body.dimension,
    easeFactor: 2.5,
    interval: 1,
    repetitions: 0,
    nextReview: Date.now(),
  };
  sm2States.set(body.dimension, sm2Update(existing, record.score));

  return NextResponse.json({ ok: true, record });
}
