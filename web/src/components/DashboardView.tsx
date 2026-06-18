'use client';

import { TrendingUp, Target, Award, Zap } from 'lucide-react';
import { RadarChart } from './RadarChart';

const DIMENSIONS = [
  { label: '架构设计', value: 7, max: 10 },
  { label: '工程实践', value: 6, max: 10 },
  { label: '模型能力', value: 8, max: 10 },
  { label: 'RAG 检索', value: 5, max: 10 },
  { label: '多 Agent', value: 6, max: 10 },
  { label: '评测体系', value: 4, max: 10 },
  { label: '全栈交付', value: 7, max: 10 },
];

const STATS = [
  { label: '总评分', value: '72', suffix: '/100', icon: Target, color: 'text-accent' },
  { label: '已诊断', value: '0', suffix: '题', icon: Award, color: 'text-emerald-500' },
  { label: '薄弱项', value: '2', suffix: '个', icon: Zap, color: 'text-amber-500' },
  { label: '进步趋势', value: '+12', suffix: '%', icon: TrendingUp, color: 'text-violet-500' },
];

export function DashboardView() {
  const totalScore = DIMENSIONS.reduce((sum, d) => sum + d.value, 0);
  const maxScore = DIMENSIONS.reduce((sum, d) => sum + d.max, 0);
  const percentage = Math.round((totalScore / maxScore) * 100);

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

        {/* Recommendations */}
        <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-card">
          <h3 className="text-sm font-semibold text-primary mb-3">改进建议</h3>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {[
              { title: '评测体系', advice: '学习 LLM-as-Judge、A/B 测试设计、自动化评测 pipeline', priority: 'high' },
              { title: 'RAG 检索', advice: '实践 Chunk 策略、Embedding 选型、混合检索 + Rerank', priority: 'high' },
              { title: '多 Agent', advice: '理解编排模式（DAG/FSM）、通信协议、上下文隔离', priority: 'medium' },
            ].map((item) => (
              <div key={item.title} className={`rounded-xl border p-3 ${
                item.priority === 'high' ? 'border-amber-200 bg-amber-50/50' : 'border-slate-200 bg-slate-50/50'
              }`}>
                <div className="flex items-center gap-2 mb-1.5">
                  <span className={`h-1.5 w-1.5 rounded-full ${
                    item.priority === 'high' ? 'bg-amber-500' : 'bg-slate-400'
                  }`} />
                  <span className="text-xs font-medium text-slate-700">{item.title}</span>
                </div>
                <p className="text-[11px] text-slate-500 leading-relaxed">{item.advice}</p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
