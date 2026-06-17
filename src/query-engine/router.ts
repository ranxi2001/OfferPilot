import type { LLMProvider } from './types.js';

export interface ProviderConfig {
  provider: LLMProvider;
  models: string[];
  defaultModel: string;
}

export class ProviderRouter {
  private configs: ProviderConfig[] = [];
  private modelMap = new Map<string, LLMProvider>();

  register(config: ProviderConfig): void {
    this.configs.push(config);
    for (const model of config.models) {
      this.modelMap.set(model, config.provider);
    }
  }

  resolve(modelOrProvider?: string): { provider: LLMProvider; model: string } {
    if (modelOrProvider) {
      const byModel = this.modelMap.get(modelOrProvider);
      if (byModel) {
        return { provider: byModel, model: modelOrProvider };
      }

      const byName = this.configs.find((c) => c.provider.name === modelOrProvider);
      if (byName) {
        return { provider: byName.provider, model: byName.defaultModel };
      }
    }

    if (this.configs.length === 0) {
      throw new Error('No providers registered. Configure at least one LLM provider (set ANTHROPIC_API_KEY, OPENAI_API_KEY, or DEEPSEEK_API_KEY).');
    }

    const first = this.configs[0];
    return { provider: first.provider, model: first.defaultModel };
  }

  listProviders(): string[] {
    return this.configs.map((c) => c.provider.name);
  }
}
