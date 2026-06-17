import { randomUUID } from 'node:crypto';
import type Database from 'better-sqlite3';
import type { MemoryEntry, MemoryQuery } from './types.js';

export class MemoryStore {
  private entries: MemoryEntry[] = [];
  private db?: Database.Database;

  constructor(db?: Database.Database) {
    this.db = db;
    if (db) {
      this.loadFromDb();
    }
  }

  add(entry: Omit<MemoryEntry, 'id' | 'createdAt' | 'lastAccessedAt' | 'accessCount'>): MemoryEntry {
    const full: MemoryEntry = {
      ...entry,
      id: randomUUID(),
      createdAt: Date.now(),
      lastAccessedAt: Date.now(),
      accessCount: 0,
    };
    this.entries.push(full);

    if (this.db) {
      this.db.prepare(`
        INSERT INTO memories (id, session_id, type, content, importance, access_count, created_at, last_accessed_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      `).run(full.id, full.sessionId, full.type, full.content, full.importance, 0, full.createdAt, full.lastAccessedAt);
    }

    return full;
  }

  query(q: MemoryQuery): MemoryEntry[] {
    let results = this.entries;

    if (q.sessionId) {
      results = results.filter((e) => e.sessionId === q.sessionId);
    }
    if (q.type) {
      results = results.filter((e) => e.type === q.type);
    }
    if (q.minImportance !== undefined) {
      results = results.filter((e) => e.importance >= q.minImportance!);
    }
    if (q.query) {
      const lower = q.query.toLowerCase();
      results = results.filter((e) => e.content.toLowerCase().includes(lower));
    }

    results.sort((a, b) => b.importance - a.importance);

    if (q.limit) {
      results = results.slice(0, q.limit);
    }

    for (const entry of results) {
      entry.lastAccessedAt = Date.now();
      entry.accessCount++;

      if (this.db) {
        this.db.prepare(`
          UPDATE memories SET access_count = ?, last_accessed_at = ? WHERE id = ?
        `).run(entry.accessCount, entry.lastAccessedAt, entry.id);
      }
    }

    return results;
  }

  getBySession(sessionId: string): MemoryEntry[] {
    return this.entries.filter((e) => e.sessionId === sessionId);
  }

  remove(id: string): boolean {
    const idx = this.entries.findIndex((e) => e.id === id);
    if (idx === -1) return false;
    this.entries.splice(idx, 1);

    if (this.db) {
      this.db.prepare('DELETE FROM memories WHERE id = ?').run(id);
    }

    return true;
  }

  size(): number {
    return this.entries.length;
  }

  private loadFromDb(): void {
    if (!this.db) return;

    const rows = this.db.prepare('SELECT * FROM memories').all() as Array<{
      id: string;
      session_id: string;
      type: string;
      content: string;
      importance: number;
      access_count: number;
      created_at: number;
      last_accessed_at: number;
    }>;

    this.entries = rows.map((row) => ({
      id: row.id,
      sessionId: row.session_id,
      type: row.type as MemoryEntry['type'],
      content: row.content,
      importance: row.importance,
      createdAt: row.created_at * 1000,
      lastAccessedAt: row.last_accessed_at * 1000,
      accessCount: row.access_count,
    }));
  }
}
