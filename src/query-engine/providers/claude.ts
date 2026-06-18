import Anthropic from '@anthropic-ai/sdk';
import type { LLMProvider, Message, StreamEvent, StreamParams, ToolSchema } from '../types.js';

export class ClaudeProvider implements LLMProvider {
  name = 'claude';
  private client: Anthropic;
  private defaultModel: string;

  constructor(apiKey?: string, defaultModel = 'claude-sonnet-4-20250514') {
    this.client = new Anthropic({ apiKey });
    this.defaultModel = defaultModel;
  }

  async *stream(params: StreamParams): AsyncIterable<StreamEvent> {
    const { model, messages, tools, maxTokens, temperature, systemPrompt, abortSignal } = params;

    const anthropicMessages = messages
      .filter((m) => m.role !== 'tool' || m.toolCallId)
      .map((m) => this.toAnthropicMessage(m));

    const requestParams: Anthropic.MessageCreateParams = {
      model,
      max_tokens: maxTokens ?? 4096,
      messages: anthropicMessages,
      stream: true,
    };

    if (systemPrompt) {
      requestParams.system = systemPrompt;
    }
    if (temperature !== undefined) {
      requestParams.temperature = temperature;
    }
    if (tools?.length) {
      requestParams.tools = tools.map((t) => this.toAnthropicTool(t));
    }

    const stream = this.client.messages.stream(requestParams, {
      signal: abortSignal,
    });

    let currentToolId = '';
    let currentToolName = '';

    for await (const event of stream) {
      switch (event.type) {
        case 'content_block_start':
          if (event.content_block.type === 'tool_use') {
            currentToolId = event.content_block.id;
            currentToolName = event.content_block.name;
            yield { type: 'tool_use_start', id: currentToolId, name: currentToolName };
          }
          break;

        case 'content_block_delta':
          if (event.delta.type === 'text_delta') {
            yield { type: 'text_delta', content: event.delta.text };
          } else if (event.delta.type === 'input_json_delta') {
            yield { type: 'tool_use_delta', input: event.delta.partial_json };
          } else if ((event.delta as any).type === 'thinking_delta') {
            yield { type: 'thinking_delta', content: (event.delta as any).thinking };
          }
          break;

        case 'content_block_stop':
          if (currentToolId) {
            yield { type: 'tool_use_end' };
            currentToolId = '';
            currentToolName = '';
          }
          break;

        case 'message_delta':
          yield {
            type: 'message_end',
            usage: {
              inputTokens: (event as any).usage?.input_tokens ?? 0,
              outputTokens: event.usage.output_tokens,
            },
            stopReason: this.mapStopReason(event.delta.stop_reason),
          };
          break;
      }
    }
  }

  async countTokens(messages: Message[], tools?: ToolSchema[], model?: string): Promise<number> {
    const result = await this.client.messages.countTokens({
      model: model ?? this.defaultModel,
      messages: messages.map((m) => this.toAnthropicMessage(m)),
      tools: tools?.map((t) => this.toAnthropicTool(t)),
    });
    return result.input_tokens;
  }

  private toAnthropicMessage(msg: Message): Anthropic.MessageParam {
    if (msg.role === 'tool') {
      return {
        role: 'user',
        content: [
          {
            type: 'tool_result',
            tool_use_id: msg.toolCallId!,
            content: msg.content ?? '',
          },
        ],
      };
    }

    if (msg.role === 'assistant' && msg.toolCalls?.length) {
      const content: Anthropic.ContentBlockParam[] = [];
      if (msg.content) {
        content.push({ type: 'text', text: msg.content });
      }
      for (const tc of msg.toolCalls) {
        content.push({ type: 'tool_use', id: tc.id, name: tc.name, input: tc.input });
      }
      return { role: 'assistant', content };
    }

    return { role: msg.role as 'user' | 'assistant', content: msg.content ?? '' };
  }

  private toAnthropicTool(tool: ToolSchema): Anthropic.Tool {
    return {
      name: tool.name,
      description: tool.description,
      input_schema: tool.parameters as Anthropic.Tool['input_schema'],
    };
  }

  private mapStopReason(reason: string | null): 'end_turn' | 'tool_use' | 'max_tokens' {
    if (reason === 'tool_use') return 'tool_use';
    if (reason === 'max_tokens') return 'max_tokens';
    return 'end_turn';
  }
}
