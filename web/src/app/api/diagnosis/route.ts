import { NextRequest, NextResponse } from 'next/server';

interface DiagnosisRecord {
  id: string;
  timestamp: number;
  dimension: string;
  score: number;
  question: string;
}

const records: DiagnosisRecord[] = [];

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

  return NextResponse.json({
    dimensionScores,
    totalAnswered,
    avgScore,
    weakDimensions,
    recent,
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

  return NextResponse.json({ ok: true, record });
}
