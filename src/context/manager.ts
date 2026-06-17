import type { Message } from '../query-engine/types.js';
import type { QueryEngine } from '../query-engine/engine.js';
import type { CompressionLevel, CompressionResult, ContextLayer, ContextWindow } from './types.js';

const DEFAULT_MAX_TOKENS = 100000;

export class ContextManager {
  private maxTokens: number;
  private layers: Partial<ContextWindow> = {};
  private queryEngine?: QueryEngine;
  private summaryModel?: string;

  constructor(opts?: number | { maxTokens?: number; queryEngine?: QueryEngine; summaryModel?: string }) {
    if (typeof opts === 'number') {
      this.maxTokens = opts;
    } else {
      this.maxTokens = opts?.maxTokens ?? DEFAULT_MAX_TOKENS;
      this.queryEngine = opts?.queryEngine;
      this.summaryModel = opts?.summaryModel;
    }
  }

  setLayer(name: keyof ContextWindow, content: string, priority?: number): void {
    this.layers[name] = {
      name,
      priority: priority ?? this.defaultPriority(name),
      content,
      tokenCount: this.estimateTokens(content),
    };
  }

  buildSystemPrompt(): string {
    const sorted = Object.values(this.layers)
      .filter((l): l is ContextLayer => !!l && !!l.content)
      .sort((a, b) => b.priority - a.priority);

    return sorted.map((l) => l.content).join('\n\n');
  }

  compress(messages: Message[], targetTokens?: number): CompressionResult {
    const target = targetTokens ?? Math.floor(this.maxTokens * 0.6);
    const originalTokens = this.estimateMessagesTokens(messages);

    if (originalTokens <= target) {
      return { messages, level: 'none', originalTokens, compressedTokens: originalTokens };
    }

    const ratio = originalTokens / target;

    if (ratio < 2) {
      const compressed = this.summarizeOlderMessages(messages);
      return {
        messages: compressed,
        level: 'summary',
        originalTokens,
        compressedTokens: this.estimateMessagesTokens(compressed),
      };
    }

    const compressed = this.aggressiveCompress(messages);
    return {
      messages: compressed,
      level: 'aggressive',
      originalTokens,
      compressedTokens: this.estimateMessagesTokens(compressed),
    };
  }

  async compressAsync(messages: Message[], targetTokens?: number): Promise<CompressionResult> {
    const target = targetTokens ?? Math.floor(this.maxTokens * 0.6);
    const originalTokens = this.estimateMessagesTokens(messages);

    if (originalTokens <= target) {
      return { messages, level: 'none', originalTokens, compressedTokens: originalTokens };
    }

    const ratio = originalTokens / target;

    if (ratio < 2) {
      const compressed = await this.llmSummarize(messages);
      return {
        messages: compressed,
        level: 'summary',
        originalTokens,
        compressedTokens: this.estimateMessagesTokens(compressed),
      };
    }

    const compressed = await this.llmSummarize(messages, true);
    return {
      messages: compressed,
      level: 'aggressive',
      originalTokens,
      compressedTokens: this.estimateMessagesTokens(compressed),
    };
  }

  private async llmSummarize(messages: Message[], aggressive = false): Promise<Message[]> {
    const keepRecent = aggressive ? 4 : Math.max(6, Math.floor(messages.length * 0.4));
    const older = messages.slice(0, -keepRecent);
    const recent = messages.slice(-keepRecent);

    if (older.length === 0) return messages;

    if (!this.queryEngine) {
      return this.summarizeOlderMessages(messages);
    }

    const conversationText = older
      .filter((m) => m.content)
      .map((m) => `[${m.role}]: ${m.content}`)
      .join('\n');

    const prompt = aggressive
      ? `以下是一段对话历史，请提取关键实体、决策和结论，用3-5个要点总结（中文）：\n\n${conversationText}`
      : `以下是一段对话历史，请保留关键信息和上下文，用简洁的摘要概括（中文），保留重要的技术细节和用户偏好：\n\n${conversationText}`;

    try {
      const response = await this.queryEngine.query({
        model: this.summaryModel,
        messages: [{ role: 'user', content: prompt }],
        systemPrompt: '你是一个对话摘要助手。只输出摘要内容，不加额外说明。',
        maxTokens: 512,
      });

      const summaryMsg: Message = {
        role: 'user',
        content: `[对话历史摘要]\n${response.content ?? ''}`,
      };

      return [summaryMsg, ...recent];
    } catch {
      return this.summarizeOlderMessages(messages);
    }
  }

  private summarizeOlderMessages(messages: Message[]): Message[] {
    const keepRecent = Math.max(6, Math.floor(messages.length * 0.4));
    const older = messages.slice(0, -keepRecent);
    const recent = messages.slice(-keepRecent);

    if (older.length === 0) return messages;

    const entities = this.extractKeyEntities(older);
    const summaryParts: string[] = [];

    if (entities.topics.length > 0) {
      summaryParts.push(`讨论主题: ${entities.topics.join(', ')}`);
    }
    if (entities.decisions.length > 0) {
      summaryParts.push(`关键决策: ${entities.decisions.join('; ')}`);
    }
    if (entities.userPreferences.length > 0) {
      summaryParts.push(`用户偏好: ${entities.userPreferences.join('; ')}`);
    }

    const lastExchanges = older.slice(-4)
      .filter((m) => m.content)
      .map((m) => `[${m.role}]: ${m.content!.slice(0, 150)}`)
      .join('\n');

    if (lastExchanges) {
      summaryParts.push(`最近交流:\n${lastExchanges}`);
    }

    const summaryMsg: Message = {
      role: 'user',
      content: `[对话历史摘要]\n${summaryParts.join('\n')}`,
    };

    return [summaryMsg, ...recent];
  }

  private extractKeyEntities(messages: Message[]): {
    topics: string[];
    decisions: string[];
    userPreferences: string[];
  } {
    const topics = new Set<string>();
    const decisions: string[] = [];
    const userPreferences: string[] = [];

    for (const msg of messages) {
      if (!msg.content) continue;
      const content = msg.content;

      if (msg.role === 'user') {
        const firstLine = content.split('\n')[0].slice(0, 60);
        if (firstLine.length > 5) topics.add(firstLine);
      }

      if (content.includes('决定') || content.includes('选择') || content.includes('确认')) {
        const sentence = content.split(/[。\n]/).find(
          (s) => s.includes('决定') || s.includes('选择') || s.includes('确认')
        );
        if (sentence && sentence.length < 100) decisions.push(sentence.trim());
      }

      if (msg.role === 'user' && (content.includes('我想') || content.includes('我要') || content.includes('我希望'))) {
        const sentence = content.split(/[。\n]/).find(
          (s) => s.includes('我想') || s.includes('我要') || s.includes('我希望')
        );
        if (sentence && sentence.length < 80) userPreferences.push(sentence.trim());
      }
    }

    return {
      topics: [...topics].slice(0, 5),
      decisions: decisions.slice(0, 3),
      userPreferences: userPreferences.slice(0, 3),
    };
  }

  private aggressiveCompress(messages: Message[]): Message[] {
    const keepRecent = 4;
    const older = messages.slice(0, -keepRecent);
    const recent = messages.slice(-keepRecent);

    const entities = this.extractKeyEntities(older);
    const parts: string[] = [];

    if (entities.topics.length > 0) parts.push(`主题: ${entities.topics.join(', ')}`);
    if (entities.decisions.length > 0) parts.push(`决策: ${entities.decisions.join('; ')}`);

    const summaryMsg: Message = {
      role: 'user',
      content: `[对话上下文: ${parts.join(' | ') || '无关键实体'}]`,
    };

    return [summaryMsg, ...recent];
  }

  private defaultPriority(name: keyof ContextWindow): number {
    const map: Record<keyof ContextWindow, number> = {
      system: 100,
      knowledge: 80,
      memory: 60,
      session: 40,
      immediate: 90,
    };
    return map[name];
  }

  private estimateTokens(text: string): number {
    return Math.ceil(text.length / 3.5);
  }

  private estimateMessagesTokens(messages: Message[]): number {
    return messages.reduce((sum, m) => sum + this.estimateTokens(m.content ?? ''), 0);
  }
}
