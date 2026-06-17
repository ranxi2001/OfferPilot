import type { EmbeddingProvider } from './search.js';

export class OpenAIEmbeddingProvider implements EmbeddingProvider {
  dimensions = 1536;
  model = 'text-embedding-3-small';
  private apiKey: string;
  private baseUrl: string;

  constructor(opts?: { apiKey?: string; model?: string; baseUrl?: string }) {
    this.apiKey = opts?.apiKey ?? process.env.OPENAI_API_KEY ?? '';
    if (opts?.model) this.model = opts.model;
    this.baseUrl = opts?.baseUrl ?? 'https://api.openai.com/v1';
    if (this.model === 'text-embedding-3-large') {
      this.dimensions = 3072;
    }
  }

  async embed(text: string): Promise<number[]> {
    const [result] = await this.embedBatch([text]);
    return result;
  }

  async embedBatch(texts: string[]): Promise<number[][]> {
    const response = await fetch(`${this.baseUrl}/embeddings`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.apiKey}`,
      },
      body: JSON.stringify({
        model: this.model,
        input: texts,
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Embedding API error (${response.status}): ${error}`);
    }

    const json = (await response.json()) as {
      data: Array<{ embedding: number[]; index: number }>;
    };

    return json.data
      .sort((a, b) => a.index - b.index)
      .map((d) => d.embedding);
  }
}
