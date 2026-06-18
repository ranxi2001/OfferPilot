export interface Message {
  role: 'user' | 'assistant' | 'tool';
  content?: string;
  toolCallId?: string;
  toolCalls?: ToolCall[];
  isError?: boolean;
}

export interface ToolCall {
  id: string;
  name: string;
  input: Record<string, unknown>;
}

export interface ToolSchema {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
}

export interface StreamParams {
  model: string;
  messages: Message[];
  tools?: ToolSchema[];
  maxTokens?: number;
  temperature?: number;
  systemPrompt?: string;
  abortSignal?: AbortSignal;
}

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
}

export type StopReason = 'end_turn' | 'tool_use' | 'max_tokens';

export type StreamEvent =
  | { type: 'text_delta'; content: string }
  | { type: 'thinking_delta'; content: string }
  | { type: 'tool_use_start'; id: string; name: string }
  | { type: 'tool_use_delta'; input: string }
  | { type: 'tool_use_end' }
  | { type: 'message_end'; usage: TokenUsage; stopReason: StopReason };

export interface ParsedResponse {
  type: 'text' | 'tool_use';
  content?: string;
  toolCalls?: ToolCall[];
  usage: TokenUsage;
  stopReason: StopReason;
}

export interface QueryParams {
  task?: string;
  model?: string;
  messages: Message[];
  tools?: ToolSchema[];
  maxTokens?: number;
  temperature?: number;
  systemPrompt?: string;
  useCache?: boolean;
  cacheTtl?: number;
  onTextDelta?: (text: string) => void;
  onThinkingDelta?: (text: string) => void;
}

export interface LLMProvider {
  name: string;
  stream(params: StreamParams): AsyncIterable<StreamEvent>;
  countTokens(messages: Message[], tools?: ToolSchema[], model?: string): Promise<number>;
}
