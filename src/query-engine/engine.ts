import type { LLMProvider, ParsedResponse, QueryParams, StreamEvent } from './types.js';
import { ProviderRouter, type ProviderConfig } from './router.js';
import { StreamCollector } from './stream.js';
import { withRetry, type RetryOptions } from './retry.js';

export interface QueryEngineOptions {
  providers: ProviderConfig[];
  defaultProvider?: string;
  retry?: RetryOptions;
}

export class QueryEngine {
  private router = new ProviderRouter();
  private retryOpts: RetryOptions;

  constructor(opts: QueryEngineOptions) {
    for (const config of opts.providers) {
      this.router.register(config);
    }
    this.retryOpts = opts.retry ?? {};
  }

  async query(params: QueryParams): Promise<ParsedResponse> {
    const { provider, model } = this.router.resolve(params.model);

    return withRetry(async () => {
      const collector = new StreamCollector();

      const stream = provider.stream({
        model,
        messages: params.messages,
        tools: params.tools,
        maxTokens: params.maxTokens,
        temperature: params.temperature,
        systemPrompt: params.systemPrompt,
      });

      for await (const event of stream) {
        collector.feed(event);
        if (event.type === 'text_delta' && params.onTextDelta) {
          params.onTextDelta(event.content);
        } else if (event.type === 'thinking_delta' && params.onThinkingDelta) {
          params.onThinkingDelta(event.content);
        }
      }

      return collector.result();
    }, this.retryOpts);
  }

  async *streamRaw(params: QueryParams): AsyncIterable<StreamEvent> {
    const { provider, model } = this.router.resolve(params.model);

    const stream = provider.stream({
      model,
      messages: params.messages,
      tools: params.tools,
      maxTokens: params.maxTokens,
      temperature: params.temperature,
      systemPrompt: params.systemPrompt,
    });

    for await (const event of stream) {
      yield event;
    }
  }

  async countTokens(params: Pick<QueryParams, 'model' | 'messages' | 'tools'>): Promise<number> {
    const { provider, model } = this.router.resolve(params.model);
    return provider.countTokens(params.messages, params.tools, model);
  }

  listProviders(): string[] {
    return this.router.listProviders();
  }
}
