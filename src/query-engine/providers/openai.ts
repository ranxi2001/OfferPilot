import OpenAI from 'openai';
import type { LLMProvider, Message, StreamEvent, StreamParams, ToolSchema } from '../types.js';

interface PendingToolCall {
  id: string;
  name: string;
  started: boolean;
  bufferedInput: string;
  order: number;
}

export class OpenAIProvider implements LLMProvider {
  name = 'openai';
  protected client: OpenAI;

  constructor(opts?: { apiKey?: string; baseURL?: string; name?: string }) {
    this.client = new OpenAI({
      apiKey: opts?.apiKey,
      baseURL: opts?.baseURL,
    });
    if (opts?.name) this.name = opts.name;
  }

  async *stream(params: StreamParams): AsyncIterable<StreamEvent> {
    const { model, messages, tools, maxTokens, temperature, systemPrompt, abortSignal } = params;

    const openaiMessages = this.buildMessages(messages, systemPrompt);

    const requestParams: OpenAI.ChatCompletionCreateParams = {
      model,
      messages: openaiMessages,
      max_tokens: maxTokens ?? 4096,
      stream: true,
      stream_options: { include_usage: true },
    };

    if (temperature !== undefined) {
      requestParams.temperature = temperature;
    }
    if (tools?.length) {
      requestParams.tools = tools.map((t) => this.toOpenAITool(t));
    }

    const stream = await this.client.chat.completions.create(requestParams, {
      signal: abortSignal,
    });

    const pendingTools = new Map<number, PendingToolCall>();
    let nextToolOrder = 0;
    let inputTokens = 0;
    let outputTokens = 0;

    for await (const chunk of stream as AsyncIterable<OpenAI.ChatCompletionChunk>) {
      if (chunk.usage) {
        inputTokens = chunk.usage.prompt_tokens;
        outputTokens = chunk.usage.completion_tokens;
      }

      const delta = chunk.choices[0]?.delta;
      if (!delta) continue;

      if (delta.content) {
        yield { type: 'text_delta', content: delta.content };
      }

      if (delta.tool_calls) {
        for (const tc of delta.tool_calls) {
          const index = typeof tc.index === 'number' ? tc.index : 0;
          let pending = pendingTools.get(index);
          if (!pending) {
            pending = { id: '', name: '', started: false, bufferedInput: '', order: nextToolOrder++ };
            pendingTools.set(index, pending);
          }

          if (tc.id) pending.id = tc.id;
          if (tc.function?.name) pending.name = tc.function.name;

          if (!pending.started && pending.id && pending.name) {
            pending.started = true;
            yield { type: 'tool_use_start', id: pending.id, name: pending.name, index };
            if (pending.bufferedInput) {
              yield { type: 'tool_use_delta', input: pending.bufferedInput, index };
              pending.bufferedInput = '';
            }
          }

          if (tc.function?.arguments) {
            if (pending.started) {
              yield { type: 'tool_use_delta', input: tc.function.arguments, index };
            } else {
              pending.bufferedInput += tc.function.arguments;
            }
          }
        }
      }

      const finishReason = chunk.choices[0]?.finish_reason;
      if (finishReason) {
        for (const [index, pending] of Array.from(pendingTools.entries()).sort((a, b) => a[1].order - b[1].order)) {
          if (pending.started) {
            yield { type: 'tool_use_end', index };
          }
        }
        pendingTools.clear();
        yield {
          type: 'message_end',
          usage: { inputTokens, outputTokens },
          stopReason: this.mapFinishReason(finishReason),
        };
      }
    }
  }

  async countTokens(_messages: Message[], _tools?: ToolSchema[], _model?: string): Promise<number> {
    const text = _messages.map((m) => m.content ?? '').join('');
    return Math.ceil(text.length / 3.5);
  }

  protected buildMessages(
    messages: Message[],
    systemPrompt?: string,
  ): OpenAI.ChatCompletionMessageParam[] {
    const result: OpenAI.ChatCompletionMessageParam[] = [];

    if (systemPrompt) {
      result.push({ role: 'system', content: systemPrompt });
    }

    for (const msg of messages) {
      if (msg.role === 'tool') {
        result.push({
          role: 'tool',
          tool_call_id: msg.toolCallId!,
          content: msg.content ?? '',
        });
      } else if (msg.role === 'assistant' && msg.toolCalls?.length) {
        result.push({
          role: 'assistant',
          content: msg.content ?? null,
          tool_calls: msg.toolCalls.map((tc) => ({
            id: tc.id,
            type: 'function' as const,
            function: { name: tc.name, arguments: JSON.stringify(tc.input) },
          })),
        });
      } else {
        result.push({
          role: msg.role as 'user' | 'assistant',
          content: msg.content ?? '',
        });
      }
    }

    return result;
  }

  private toOpenAITool(tool: ToolSchema): OpenAI.ChatCompletionTool {
    return {
      type: 'function',
      function: {
        name: tool.name,
        description: tool.description,
        parameters: tool.parameters,
      },
    };
  }

  private mapFinishReason(reason: string): 'end_turn' | 'tool_use' | 'max_tokens' {
    if (reason === 'tool_calls') return 'tool_use';
    if (reason === 'length') return 'max_tokens';
    return 'end_turn';
  }
}
