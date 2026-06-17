import type Database from 'better-sqlite3';
import type { KnowledgeEntry, SearchOptions, SearchResult } from './types.js';

export interface EmbeddingProvider {
  embed(text: string): Promise<number[]>;
  embedBatch(texts: string[]): Promise<number[][]>;
  dimensions: number;
  model: string;
}

export class KnowledgeSearch {
  private db: Database.Database;
  private embeddingProvider?: EmbeddingProvider;

  constructor(db: Database.Database, embeddingProvider?: EmbeddingProvider) {
    this.db = db;
    this.embeddingProvider = embeddingProvider;
  }

  search(opts: SearchOptions): SearchResult[] {
    const { query, dimension, limit = 5, method = 'hybrid' } = opts;

    if (method === 'fts') return this.ftsSearch(query, dimension, limit);
    if (method === 'embedding') return this.embeddingSearch(query, dimension, limit);

    const ftsResults = this.ftsSearch(query, dimension, limit);
    const embResults = this.embeddingSearch(query, dimension, limit);

    return this.mergeResults(ftsResults, embResults, limit);
  }

  async searchAsync(opts: SearchOptions): Promise<SearchResult[]> {
    const { query, dimension, limit = 5, method = 'hybrid' } = opts;

    if (method === 'fts') return this.ftsSearch(query, dimension, limit);
    if (method === 'embedding') return this.embeddingSearchAsync(query, dimension, limit);

    const ftsResults = this.ftsSearch(query, dimension, limit);
    const embResults = await this.embeddingSearchAsync(query, dimension, limit);

    return this.mergeResults(ftsResults, embResults, limit);
  }

  private ftsSearch(query: string, dimension: string | undefined, limit: number): SearchResult[] {
    const ftsQuery = query
      .replace(/[^\w一-鿿\s]/g, ' ')
      .split(/\s+/)
      .filter(Boolean)
      .map((term) => `"${term}"`)
      .join(' OR ');

    if (!ftsQuery) return [];

    let sql = `
      SELECT k.*, rank
      FROM knowledge_fts f
      JOIN knowledge k ON k.rowid = f.rowid
      WHERE knowledge_fts MATCH ?
    `;
    const params: unknown[] = [ftsQuery];

    if (dimension) {
      sql += ` AND k.dimension = ?`;
      params.push(dimension);
    }

    sql += ` ORDER BY rank LIMIT ?`;
    params.push(limit);

    const rows = this.db.prepare(sql).all(...params) as (Record<string, unknown> & { rank: number })[];

    return rows.map((row) => ({
      entry: this.rowToEntry(row),
      score: Math.abs(row.rank),
      matchType: 'fts' as const,
    }));
  }

  private embeddingSearch(query: string, dimension: string | undefined, limit: number): SearchResult[] {
    if (!this.embeddingProvider) {
      return this.likeFallback(query, dimension, limit);
    }
    return this.likeFallback(query, dimension, limit);
  }

  private async embeddingSearchAsync(query: string, dimension: string | undefined, limit: number): Promise<SearchResult[]> {
    if (!this.embeddingProvider) {
      return this.likeFallback(query, dimension, limit);
    }

    const queryVector = await this.embeddingProvider.embed(query);

    let sql = `
      SELECT e.vector, e.knowledge_id, k.*
      FROM embeddings e
      JOIN knowledge k ON k.id = e.knowledge_id
    `;
    const params: unknown[] = [];

    if (dimension) {
      sql += ` WHERE k.dimension = ?`;
      params.push(dimension);
    }

    const rows = this.db.prepare(sql).all(...params) as Array<Record<string, unknown> & { vector: Buffer; knowledge_id: string }>;

    if (rows.length === 0) {
      return this.likeFallback(query, dimension, limit);
    }

    const scored = rows.map((row) => {
      const storedVector = this.deserializeVector(row.vector);
      const similarity = this.cosineSimilarity(queryVector, storedVector);
      return { row, similarity };
    });

    scored.sort((a, b) => b.similarity - a.similarity);

    return scored.slice(0, limit).map((item) => ({
      entry: this.rowToEntry(item.row),
      score: item.similarity,
      matchType: 'embedding' as const,
    }));
  }

  async indexEmbeddings(ids?: string[]): Promise<number> {
    if (!this.embeddingProvider) {
      throw new Error('No embedding provider configured');
    }

    let sql = 'SELECT * FROM knowledge WHERE id NOT IN (SELECT knowledge_id FROM embeddings)';
    const params: unknown[] = [];

    if (ids && ids.length > 0) {
      const placeholders = ids.map(() => '?').join(',');
      sql = `SELECT * FROM knowledge WHERE id IN (${placeholders})`;
      params.push(...ids);
    }

    const rows = this.db.prepare(sql).all(...params) as Record<string, unknown>[];
    if (rows.length === 0) return 0;

    const texts = rows.map((row) => {
      const entry = this.rowToEntry(row);
      return `${entry.title}\n${entry.content}\n${entry.question ?? ''}`;
    });

    const batchSize = 100;
    let indexed = 0;
    const insert = this.db.prepare(`
      INSERT OR REPLACE INTO embeddings (knowledge_id, vector, model, created_at)
      VALUES (?, ?, ?, unixepoch())
    `);

    for (let i = 0; i < texts.length; i += batchSize) {
      const batch = texts.slice(i, i + batchSize);
      const vectors = await this.embeddingProvider.embedBatch(batch);

      const tx = this.db.transaction(() => {
        for (let j = 0; j < vectors.length; j++) {
          const knowledgeId = (rows[i + j] as { id: string }).id;
          insert.run(knowledgeId, this.serializeVector(vectors[j]), this.embeddingProvider!.model);
        }
      });
      tx();

      indexed += vectors.length;
    }

    return indexed;
  }

  private likeFallback(query: string, dimension: string | undefined, limit: number): SearchResult[] {
    let sql = `SELECT * FROM knowledge WHERE content LIKE ?`;
    const params: unknown[] = [`%${query.slice(0, 20)}%`];

    if (dimension) {
      sql += ` AND dimension = ?`;
      params.push(dimension);
    }

    sql += ` LIMIT ?`;
    params.push(limit);

    const rows = this.db.prepare(sql).all(...params) as Record<string, unknown>[];

    return rows.map((row, i) => ({
      entry: this.rowToEntry(row),
      score: 1 - i * 0.1,
      matchType: 'embedding' as const,
    }));
  }

  private cosineSimilarity(a: number[], b: number[]): number {
    let dotProduct = 0;
    let normA = 0;
    let normB = 0;

    for (let i = 0; i < a.length; i++) {
      dotProduct += a[i] * b[i];
      normA += a[i] * a[i];
      normB += b[i] * b[i];
    }

    const denom = Math.sqrt(normA) * Math.sqrt(normB);
    if (denom === 0) return 0;
    return dotProduct / denom;
  }

  private serializeVector(vector: number[]): Buffer {
    const buf = Buffer.alloc(vector.length * 4);
    for (let i = 0; i < vector.length; i++) {
      buf.writeFloatLE(vector[i], i * 4);
    }
    return buf;
  }

  private deserializeVector(buf: Buffer): number[] {
    const vector: number[] = [];
    for (let i = 0; i < buf.length; i += 4) {
      vector.push(buf.readFloatLE(i));
    }
    return vector;
  }

  private mergeResults(fts: SearchResult[], emb: SearchResult[], limit: number): SearchResult[] {
    const seen = new Set<string>();
    const merged: SearchResult[] = [];

    const all = [...fts, ...emb].sort((a, b) => b.score - a.score);

    for (const result of all) {
      if (seen.has(result.entry.id)) continue;
      seen.add(result.entry.id);
      merged.push({ ...result, matchType: 'hybrid' });
      if (merged.length >= limit) break;
    }

    return merged;
  }

  insert(entry: KnowledgeEntry): void {
    this.db.prepare(`
      INSERT OR REPLACE INTO knowledge (id, title, dimension, content, source_file, question, expert_answer, novice_answer, tags)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      entry.id,
      entry.title,
      entry.dimension,
      entry.content,
      entry.sourceFile ?? null,
      entry.question ?? null,
      entry.expertAnswer ?? null,
      entry.noviceAnswer ?? null,
      entry.tags?.join(',') ?? null,
    );
  }

  bulkInsert(entries: KnowledgeEntry[]): void {
    const insert = this.db.prepare(`
      INSERT OR REPLACE INTO knowledge (id, title, dimension, content, source_file, question, expert_answer, novice_answer, tags)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);

    const tx = this.db.transaction((items: KnowledgeEntry[]) => {
      for (const entry of items) {
        insert.run(
          entry.id,
          entry.title,
          entry.dimension,
          entry.content,
          entry.sourceFile ?? null,
          entry.question ?? null,
          entry.expertAnswer ?? null,
          entry.noviceAnswer ?? null,
          entry.tags?.join(',') ?? null,
        );
      }
    });

    tx(entries);
  }

  count(): number {
    const row = this.db.prepare('SELECT COUNT(*) as cnt FROM knowledge').get() as { cnt: number };
    return row.cnt;
  }

  private rowToEntry(row: Record<string, unknown>): KnowledgeEntry {
    return {
      id: row.id as string,
      title: row.title as string,
      dimension: row.dimension as string,
      content: row.content as string,
      sourceFile: row.source_file as string | undefined,
      question: row.question as string | undefined,
      expertAnswer: row.expert_answer as string | undefined,
      noviceAnswer: row.novice_answer as string | undefined,
      tags: (row.tags as string)?.split(',').filter(Boolean),
    };
  }
}
