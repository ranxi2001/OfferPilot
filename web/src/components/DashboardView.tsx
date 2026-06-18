'use client';

import { useEffect, useState } from 'react';
import { TrendingUp, Target, Award, Zap } from 'lucide-react';
import { RadarChart } from './RadarChart';

const DIMENSION_LABELS: Record<string, string> = {
  architecture: '架构设计',
  engineering: '工程实践',
  model: '模型能力',
  rag: 'RAG 检索',
  'multi-agent': '多 Agent',
  evaluation: '评测体系',
  'full-stack': '全栈交付',
};

interface DashboardData {
  dimensionScores: { dimension: string; score: number; count: number }[];
  totalAnswered: number;
  avgScore: number;
  weakDimensions: string[];
  recent: { id: string; dimension: string; score: number; question: string; timestamp: number }[];
  reviewPriority?: { dimension: string; urgency: number; daysUntilReview: number | null }[];
}

export function DashboardView() {
  const [data, setData] = useState<DashboardData | null>(null);

  useEffect(() => {
    fetch('/api/diagnosis').then((r) => r.json()).then(setData);
  }, []);

  const DIMENSIONS = data
    ? data.dimensionScores.map((d) => ({
        label: DIMENSION_LABELS[d.dimension] ?? d.dimension,
        value: d.score || 0,
        max: 10,
      }))
    : Object.values(DIMENSION_LABELS).map((label) => ({ label, value: 0, max: 10 }));

  const totalAnswered = data?.totalAnswered ?? 0;
  const avgScore = data?.avgScore ?? 0;
  const weakCount = data?.weakDimensions.length ?? 0;

  const STATS = [
    { label: '平均分', value: String(avgScore), suffix: '/10', icon: Target, color: 'text-accent' },
    { label: '已诊断', value: String(totalAnswered), suffix: '题', icon: Award, color: 'text-emerald-500' },
    { label: '薄弱项', value: String(weakCount), suffix: '个', icon: Zap, color: 'text-amber-500' },
    { label: '总分', value: String(DIMENSIONS.reduce((s, d) => s + d.value, 0)), suffix: '/70', icon: TrendingUp, color: 'text-violet-500' },
  ];

  const totalScore = DIMENSIONS.reduce((sum, d) => sum + d.value, 0);
  const maxScore = DIMENSIONS.reduce((sum, d) => sum + d.max, 0);
  const percentage = maxScore > 0 ? Math.round((totalScore / maxScore) * 100) : 0;

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-5xl space-y-6">
        {/* Stats row */}
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {STATS.map((stat) => {
            const Icon = stat.icon;
            return (
              <div key={stat.label} className="rounded-2xl border border-slate-200/80 bg-white p-4 shadow-card">
                <div className="flex items-center gap-2 mb-2">
                  <Icon size={14} className={stat.color} />
                  <span className="text-xs text-slate-500">{stat.label}</span>
                </div>
                <div className="flex items-baseline gap-1">
                  <span className="text-2xl font-bold text-primary">{stat.value}</span>
                  <span className="text-sm text-slate-400">{stat.suffix}</span>
                </div>
              </div>
            );
          })}
        </div>

        {/* Main content */}
        <div className="grid gap-6 lg:grid-cols-2">
          {/* Radar chart card */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
            <h3 className="text-sm font-semibold text-primary mb-1">能力雷达图</h3>
            <p className="text-xs text-slate-400 mb-4">7 维度面试能力评估</p>
            <div className="flex justify-center">
              <RadarChart dimensions={DIMENSIONS} size={300} />
            </div>
          </div>

          {/* Dimension details */}
          <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
            <h3 className="text-sm font-semibold text-primary mb-1">维度详情</h3>
            <p className="text-xs text-slate-400 mb-4">各维度得分与建议</p>
            <div className="space-y-3">
              {DIMENSIONS.map((d) => {
                const pct = (d.value / d.max) * 100;
                const isWeak = pct < 60;
                return (
                  <div key={d.label} className="group">
                    <div className="flex items-center justify-between mb-1">
                      <span className={`text-xs font-medium ${isWeak ? 'text-amber-600' : 'text-slate-600'}`}>
                        {d.label}
                        {isWeak && <span className="ml-1.5 text-[10px] text-amber-500">需加强</span>}
                      </span>
                      <span className="text-xs text-slate-400">{d.value}/{d.max}</span>
                    </div>
                    <div className="h-2 rounded-full bg-slate-100 overflow-hidden">
                      <div
                        className={`h-full rounded-full transition-all ${
                          isWeak ? 'bg-gradient-to-r from-amber-400 to-amber-500' : 'bg-gradient-to-r from-accent to-cyan'
                        }`}
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Overall */}
            <div className="mt-6 pt-4 border-t border-slate-100">
              <div className="flex items-center justify-between">
                <span className="text-sm font-semibold text-primary">面试准备度</span>
                <div className="flex items-center gap-2">
                  <div className="h-2.5 w-24 rounded-full bg-slate-100 overflow-hidden">
                    <div
                      className="h-full rounded-full bg-gradient-to-r from-accent via-cyan to-emerald-400"
                      style={{ width: `${percentage}%` }}
                    />
                  </div>
                  <span className="text-sm font-bold text-accent">{percentage}%</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Learning Path */}
        <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
          <h3 className="text-sm font-semibold text-primary mb-1">学习路径推荐</h3>
          <p className="text-xs text-slate-400 mb-4">根据薄弱维度自动生成，完成后重新诊断更新</p>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {getLearningPath(DIMENSIONS).map((item) => (
              <div key={item.title} className={`rounded-xl border p-3 ${
                item.priority === 'high' ? 'border-amber-200 bg-amber-50/50' : 'border-slate-200 bg-slate-50/50'
              }`}>
                <div className="flex items-center gap-2 mb-1.5">
                  <span className={`h-1.5 w-1.5 rounded-full ${
                    item.priority === 'high' ? 'bg-amber-500' : 'bg-slate-400'
                  }`} />
                  <span className="text-xs font-medium text-slate-700">{item.title}</span>
                  <span className="text-[10px] text-slate-400 ml-auto">{item.step}</span>
                </div>
                <p className="text-[11px] text-slate-500 leading-relaxed">{item.advice}</p>
              </div>
            ))}
          </div>
        </div>

        {/* SM-2 Review Priority */}
        {data && data.reviewPriority && data.reviewPriority.length > 0 && (
          <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
            <h3 className="text-sm font-semibold text-primary mb-1">间隔复习提醒</h3>
            <p className="text-xs text-slate-400 mb-4">基于 SM-2 算法自动排期，优先复习薄弱+到期维度</p>
            <div className="space-y-2">
              {data.reviewPriority.slice(0, 5).map((r) => (
                <div key={r.dimension} className="flex items-center justify-between rounded-lg bg-surface-muted px-3 py-2">
                  <div className="flex items-center gap-2">
                    <span className={`h-2 w-2 rounded-full ${
                      r.daysUntilReview !== null && r.daysUntilReview <= 0 ? 'bg-red-400' : r.urgency > 3 ? 'bg-amber-400' : 'bg-slate-300'
                    }`} />
                    <span className="text-xs font-medium text-slate-700">
                      {DIMENSION_LABELS[r.dimension] ?? r.dimension}
                    </span>
                  </div>
                  <span className={`text-[11px] ${
                    r.daysUntilReview !== null && r.daysUntilReview <= 0 ? 'text-red-500 font-medium' : 'text-slate-400'
                  }`}>
                    {r.daysUntilReview === null ? '—' : r.daysUntilReview <= 0 ? '今天复习' : `${r.daysUntilReview} 天后`}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Recent history */}
        {data && data.recent.length > 0 && (
          <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
            <h3 className="text-sm font-semibold text-primary mb-3">最近诊断记录</h3>
            <div className="space-y-2">
              {data.recent.map((r) => (
                <div key={r.id} className="flex items-center justify-between rounded-lg bg-surface-muted px-3 py-2">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-[11px] text-accent font-medium shrink-0">
                      {DIMENSION_LABELS[r.dimension] ?? r.dimension}
                    </span>
                    <span className="text-[11px] text-slate-400 truncate">{r.question}</span>
                  </div>
                  <span className={`text-xs font-bold shrink-0 ml-2 ${r.score >= 7 ? 'text-emerald-500' : r.score >= 5 ? 'text-accent' : 'text-amber-500'}`}>
                    {r.score}/10
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

const LEARNING_ADVICE: Record<string, { steps: string[]; advice: string[] }> = {
  '架构设计': {
    steps: ['理解 10 层 Harness 架构', '手写 Agent Loop', '设计 Provider 抽象层'],
    advice: ['学习 Agent 系统分层设计', '实践 Query Engine 多 Provider 路由', '理解 abort/budget 控制机制'],
  },
  '工程实践': {
    steps: ['Hook 管线设计', '结构化日志', 'CI/CD 流水线'],
    advice: ['实现 pre-tool/post-tool Hook', '搭建 GitHub Actions CI', '实践 Docker 多阶段构建'],
  },
  '模型能力': {
    steps: ['Prompt 工程', 'Temperature 调参', '模型评估对比'],
    advice: ['掌握 System Prompt 设计模式', '理解 token 计费与上下文管理', '学会 A/B 对比不同模型效果'],
  },
  'RAG 检索': {
    steps: ['Chunk 策略', 'Embedding 选型', '混合检索 + Rerank'],
    advice: ['实践固定/语义/递归 Chunk 切分', '对比 BGE-M3 vs OpenAI embedding', '实现 FTS + Vector 混合排序'],
  },
  '多 Agent': {
    steps: ['理解编排模式', '上下文隔离', '通信协议'],
    advice: ['学习 DAG/FSM 编排模式', '实现 Sub-agent 并发池', '设计 Agent 间的上下文传递机制'],
  },
  '评测体系': {
    steps: ['LLM-as-Judge', '自动化评测', '对比测试'],
    advice: ['用 LLM 评分模型打分诊断质量', '建立回归测试集', '设计 A/B 测试框架'],
  },
  '全栈交付': {
    steps: ['SSE 流式', '前端状态管理', '端到端部署'],
    advice: ['实现 Server-Sent Events 实时推送', '掌握 Next.js 14 App Router', '实践 Docker Compose 全栈部署'],
  },
};

function getLearningPath(dimensions: { label: string; value: number; max: number }[]) {
  const sorted = [...dimensions].sort((a, b) => (a.value / a.max) - (b.value / b.max));
  return sorted.slice(0, 4).map((d, i) => {
    const advice = LEARNING_ADVICE[d.label];
    const stepIdx = Math.min(i, (advice?.steps.length ?? 1) - 1);
    return {
      title: d.label,
      priority: d.value / d.max < 0.5 ? 'high' as const : 'medium' as const,
      step: advice?.steps[stepIdx] ?? '基础学习',
      advice: advice?.advice[stepIdx] ?? '加强该方向的练习',
    };
  });
}
