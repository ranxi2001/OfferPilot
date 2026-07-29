import type { ParsedResponse, StreamEvent, ToolCall, TokenUsage } from './types.js';

interface ToolState {
  id: string;
  name: string;
  input: string;
  order: number;
}

export class StreamCollector {
  private text = '';
  private toolCalls: ToolCall[] = [];
  private toolStates = new Map<string, ToolState>();
  private currentToolKey: string | null = null;
  private nextToolOrder = 0;
  private usage: TokenUsage = { inputTokens: 0, outputTokens: 0 };
  private stopReason: 'end_turn' | 'tool_use' | 'max_tokens' = 'end_turn';

  feed(event: StreamEvent): void {
    switch (event.type) {
      case 'text_delta':
        this.text += event.content;
        break;
      case 'tool_use_start':
        this.startTool(event.index, event.id, event.name);
        break;
      case 'tool_use_delta':
        this.appendToolInput(event.index, event.input);
        break;
      case 'tool_use_end':
        this.endTool(event.index);
        break;
      case 'message_end':
        this.usage = event.usage;
        this.stopReason = event.stopReason;
        break;
    }
  }

  result(): ParsedResponse {
    return {
      type: this.toolCalls.length > 0 ? 'tool_use' : 'text',
      content: this.text || undefined,
      toolCalls: this.toolCalls.length > 0 ? this.toolCalls : undefined,
      usage: this.usage,
      stopReason: this.stopReason,
    };
  }

  private startTool(index: number | undefined, id: string, name: string): void {
    const key = index === undefined ? `seq:${this.nextToolOrder}` : `idx:${index}`;
    this.currentToolKey = key;
    this.toolStates.set(key, {
      id,
      name,
      input: '',
      order: this.nextToolOrder++,
    });
  }

  private appendToolInput(index: number | undefined, input: string): void {
    const key = index === undefined ? this.currentToolKey : `idx:${index}`;
    if (!key) return;

    const state = this.toolStates.get(key);
    if (!state) return;

    state.input += input;
  }

  private endTool(index: number | undefined): void {
    const key = index === undefined ? this.currentToolKey : `idx:${index}`;
    if (!key) return;

    const state = this.toolStates.get(key);
    if (!state) return;

    this.toolCalls.push({
      id: state.id,
      name: state.name,
      input: this.parseInput(state.input),
    });

    this.toolStates.delete(key);
    if (this.currentToolKey === key) {
      this.currentToolKey = null;
    }
  }

  private parseInput(raw: string): Record<string, unknown> {
    if (!raw) return {};
    try {
      return JSON.parse(raw);
    } catch {
      return { _raw: raw };
    }
  }
}
