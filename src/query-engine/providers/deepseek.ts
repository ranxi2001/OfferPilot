import { OpenAIProvider } from './openai.js';

export class DeepSeekProvider extends OpenAIProvider {
  constructor(apiKey?: string, baseURL?: string) {
    super({
      apiKey: apiKey ?? process.env.DEEPSEEK_API_KEY,
      baseURL: baseURL ?? process.env.DEEPSEEK_BASE_URL ?? 'https://api.deepseek.com',
      name: 'deepseek',
    });
  }
}
