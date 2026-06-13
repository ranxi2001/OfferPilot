import { describe, it, expect } from 'vitest';
import { DefectAnalyzer } from '../../src/realtime/defect-analyzer';
import { RealtimeInterviewSession } from '../../src/realtime/session-manager';
import { TTSEngine } from '../../src/realtime/tts';

describe('DefectAnalyzer', () => {
  const analyzer = new DefectAnalyzer();

  it('detects too_short answers', () => {
    const defects = analyzer.analyze({ question: '什么是 ReAct？', answer: '就是循环调用', elapsedMs: 3000 });
    expect(defects.some((d) => d.type === 'too_short')).toBe(true);
  });

  it('detects no_structure in long unstructured answers', () => {
    const longAnswer = 'ReAct模式就是让模型可以使用工具来获取外部信息然后继续推理得到结果这样可以弥补模型本身知识的不足同时也能做一些计算类的任务不过实现起来需要注意循环次数的控制避免无限循环还有错误处理也很重要需要考虑到各种边界条件和异常情况的处理比如网络超时和API返回格式异常这些都要仔细考虑到';
    const defects = analyzer.analyze({ question: '什么是 ReAct？', answer: longAnswer, elapsedMs: 5000 });
    expect(defects.some((d) => d.type === 'no_structure')).toBe(true);
  });

  it('detects filler_words', () => {
    const answer = '嗯那个就是说ReAct嗯就是一种模式那个让模型嗯可以调用工具然后就处理结果对吧然后就返回给用户对吧';
    const defects = analyzer.analyze({ question: '什么是 ReAct？', answer, elapsedMs: 5000 });
    expect(defects.some((d) => d.type === 'filler_words')).toBe(true);
  });

  it('detects too_vague expressions', () => {
    const answer = '可能是某些情况下大概需要用一些工具来差不多实现好像是这样的功能可能还需要一些额外的配置差不多就这样';
    const defects = analyzer.analyze({ question: '如何设计工具调用层？', answer, elapsedMs: 3000 });
    expect(defects.some((d) => d.type === 'too_vague')).toBe(true);
  });

  it('returns empty for well-structured answer', () => {
    const goodAnswer = '首先，ReAct模式的核心是将推理和行动交替执行。因为模型本身知识有限，需要外部工具补充。比如在我之前的项目中，我们实现了一个3步循环：1. 模型分析问题 2. 调用搜索工具 3. 整合结果返回。其次，工程上需要注意循环次数限制，一般设置最多5轮避免无限循环。';
    const defects = analyzer.analyze({ question: '什么是 ReAct？', answer: goodAnswer, elapsedMs: 8000 });
    expect(defects.length).toBe(0);
  });
});

describe('RealtimeInterviewSession', () => {
  it('starts and returns first question', () => {
    const session = new RealtimeInterviewSession(['Q1', 'Q2', 'Q3']);
    const q = session.start();
    expect(q).toBe('Q1');
    expect(session.state).toBe('questioning');
    expect(session.progress).toEqual({ asked: 0, total: 3 });
  });

  it('submits answer and receives defect analysis', () => {
    const session = new RealtimeInterviewSession(['什么是微服务？']);
    session.start();
    const { defects, summary } = session.submitAnswer('就那样');
    expect(defects.length).toBeGreaterThan(0);
    expect(summary).toContain('问题');
  });

  it('advances through all questions and generates report', () => {
    const session = new RealtimeInterviewSession(['Q1', 'Q2']);
    session.start();
    session.submitAnswer('第一个问题的很好的回答，首先我认为这个问题涉及到多个层面。比如在实际项目中我们需要考虑性能和可维护性。因为底层实现决定了上层架构。');
    session.nextQuestion();
    session.submitAnswer('简短');
    const report = session.getReport();
    expect(report.totalDefects).toBeGreaterThanOrEqual(1);
    expect(report.overallScore).toBeGreaterThanOrEqual(1);
    expect(report.overallScore).toBeLessThanOrEqual(10);
  });

  it('returns empty string when all questions asked', () => {
    const session = new RealtimeInterviewSession(['只有一题']);
    session.start();
    session.submitAnswer('回答');
    const next = session.nextQuestion();
    expect(next).toBe('');
  });
});

describe('TTSEngine', () => {
  it('generates SSML for browser provider', async () => {
    const engine = new TTSEngine({ provider: 'browser' });
    const output = await engine.synthesize('你好');
    expect(output.text).toBe('你好');
    expect(output.ssml).toContain('<speak');
    expect(output.ssml).toContain('你好');
  });

  it('generates audioUrl for edge-tts provider', async () => {
    const engine = new TTSEngine({ provider: 'edge-tts' });
    const output = await engine.synthesize('测试语音');
    expect(output.audioUrl).toContain('/api/tts/edge');
    expect(output.audioUrl).toContain(encodeURIComponent('测试语音'));
  });

  it('generates audioUrl for openai-tts provider', async () => {
    const engine = new TTSEngine({ provider: 'openai-tts' });
    const output = await engine.synthesize('hello');
    expect(output.audioUrl).toContain('/api/tts/openai');
  });
});
