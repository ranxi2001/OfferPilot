import { describe, it, expect } from 'vitest';
import { StreamCollector } from '../../src/query-engine/stream.js';

describe('StreamCollector', () => {
  it('should collect text deltas into a single response', () => {
    const collector = new StreamCollector();

    collector.feed({ type: 'text_delta', content: 'Hello' });
    collector.feed({ type: 'text_delta', content: ' world' });
    collector.feed({
      type: 'message_end',
      usage: { inputTokens: 10, outputTokens: 5 },
      stopReason: 'end_turn',
    });

    const result = collector.result();
    expect(result.type).toBe('text');
    expect(result.content).toBe('Hello world');
    expect(result.usage.inputTokens).toBe(10);
    expect(result.stopReason).toBe('end_turn');
  });

  it('should collect tool use events', () => {
    const collector = new StreamCollector();

    collector.feed({ type: 'tool_use_start', id: 'tc_1', name: 'search_knowledge' });
    collector.feed({ type: 'tool_use_delta', input: '{"query":' });
    collector.feed({ type: 'tool_use_delta', input: '"react"}' });
    collector.feed({ type: 'tool_use_end' });
    collector.feed({
      type: 'message_end',
      usage: { inputTokens: 20, outputTokens: 15 },
      stopReason: 'tool_use',
    });

    const result = collector.result();
    expect(result.type).toBe('tool_use');
    expect(result.toolCalls).toHaveLength(1);
    expect(result.toolCalls![0].name).toBe('search_knowledge');
    expect(result.toolCalls![0].input).toEqual({ query: 'react' });
    expect(result.stopReason).toBe('tool_use');
  });

  it('should handle mixed text and tool calls', () => {
    const collector = new StreamCollector();

    collector.feed({ type: 'text_delta', content: 'Let me search' });
    collector.feed({ type: 'tool_use_start', id: 'tc_2', name: 'diagnose_answer' });
    collector.feed({ type: 'tool_use_delta', input: '{"question":"test","answer":"ans"}' });
    collector.feed({ type: 'tool_use_end' });
    collector.feed({
      type: 'message_end',
      usage: { inputTokens: 30, outputTokens: 25 },
      stopReason: 'tool_use',
    });

    const result = collector.result();
    expect(result.type).toBe('tool_use');
    expect(result.content).toBe('Let me search');
    expect(result.toolCalls![0].input).toEqual({ question: 'test', answer: 'ans' });
  });

  it('should handle malformed JSON in tool input', () => {
    const collector = new StreamCollector();

    collector.feed({ type: 'tool_use_start', id: 'tc_3', name: 'test' });
    collector.feed({ type: 'tool_use_delta', input: 'not json' });
    collector.feed({ type: 'tool_use_end' });
    collector.feed({
      type: 'message_end',
      usage: { inputTokens: 5, outputTokens: 3 },
      stopReason: 'tool_use',
    });

    const result = collector.result();
    expect(result.toolCalls![0].input).toEqual({ _raw: 'not json' });
  });

  it('should keep indexed parallel tool inputs separate', () => {
    const collector = new StreamCollector();

    collector.feed({ type: 'tool_use_start', id: 'tc_1', name: 'search_knowledge', index: 0 });
    collector.feed({ type: 'tool_use_start', id: 'tc_2', name: 'diagnose_answer', index: 1 });
    collector.feed({ type: 'tool_use_delta', input: '{"query":', index: 0 });
    collector.feed({ type: 'tool_use_delta', input: '{"question":', index: 1 });
    collector.feed({ type: 'tool_use_delta', input: '"react"}', index: 0 });
    collector.feed({ type: 'tool_use_delta', input: '"q","answer":"a"}', index: 1 });
    collector.feed({ type: 'tool_use_end', index: 0 });
    collector.feed({ type: 'tool_use_end', index: 1 });
    collector.feed({
      type: 'message_end',
      usage: { inputTokens: 20, outputTokens: 15 },
      stopReason: 'tool_use',
    });

    const result = collector.result();
    expect(result.type).toBe('tool_use');
    expect(result.toolCalls).toHaveLength(2);
    expect(result.toolCalls![0]).toMatchObject({
      id: 'tc_1',
      name: 'search_knowledge',
      input: { query: 'react' },
    });
    expect(result.toolCalls![1]).toMatchObject({
      id: 'tc_2',
      name: 'diagnose_answer',
      input: { question: 'q', answer: 'a' },
    });
  });
});
