import OpenAI from 'openai';
import type { LLMProvider, Message, StreamEvent, StreamParams, ToolSchema } from '../types.js';

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

    let currentToolId = '';
    let currentToolName = '';
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
          if (tc.id && tc.function?.name) {
            if (currentToolId) {
              yield { type: 'tool_use_end' };
            }
            currentToolId = tc.id;
            currentToolName = tc.function.name;
            yield { type: 'tool_use_start', id: currentToolId, name: currentToolName };
          }
          if (tc.function?.arguments) {
            yield { type: 'tool_use_delta', input: tc.function.arguments };
          }
        }
      }

      const finishReason = chunk.choices[0]?.finish_reason;
      if (finishReason) {
        if (currentToolId) {
          yield { type: 'tool_use_end' };
          currentToolId = '';
        }
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
