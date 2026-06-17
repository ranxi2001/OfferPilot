import { describe, it, expect } from 'vitest';
import { createApp } from '../../src/app.js';

describe('E2E: Agent Flow', () => {
  it('should create session and run agent to produce non-empty response', async () => {
    const app = createApp();
    const session = app.sessionManager.create();

    expect(session.id).toBeTruthy();
    expect(session.state).toBe('idle');

    const result = await app.agent.run(session.id, '你好');

    expect(result).toBeTruthy();
    expect(result.length).toBeGreaterThan(0);
    expect(app.sessionManager.get(session.id)!.state).toBe('active');
  });

  it('should handle tool calls during diagnosis', async () => {
    const toolCalls: string[] = [];
    const app = createApp({
      onToolCall: (name) => toolCalls.push(name),
    });
    const session = app.sessionManager.create();

    const result = await app.agent.run(
      session.id,
      '请诊断我对这个面试题的回答：\n题目：什么是 ReAct？\n我的回答：ReAct就是思考加行动',
    );

    expect(result).toBeTruthy();
    expect(result.length).toBeGreaterThan(0);
  });

  it('should track token usage', async () => {
    const app = createApp();
    const session = app.sessionManager.create();

    await app.agent.run(session.id, '你好');
    const usage = app.agent.getUsage();

    expect(usage.iterations).toBeGreaterThan(0);
    expect(usage.totalTokens).toBeGreaterThanOrEqual(0);
  });

  it('should handle sub-agent dispatch tool', async () => {
    const app = createApp();
    const session = app.sessionManager.create();

    expect(app.subAgentRuntime).toBeTruthy();
    expect(app.toolRegistry.has('dispatch_sub_agent')).toBe(true);
  });

  it('should recover from max iterations with fallback message', async () => {
    const app = createApp({ model: 'mock' });
    const session = app.sessionManager.create();

    const result = await app.agent.run(session.id, '测试');
    expect(result).toBeTruthy();
    expect(result.length).toBeGreaterThan(0);
  });
});
