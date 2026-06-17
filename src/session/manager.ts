import { randomUUID } from 'node:crypto';
import type Database from 'better-sqlite3';
import type { Message } from '../query-engine/types.js';
import type { Checkpoint, Session, SessionState } from './types.js';

export class SessionManager {
  private sessions = new Map<string, Session>();
  private checkpoints = new Map<string, Checkpoint[]>();
  private db?: Database.Database;

  constructor(db?: Database.Database) {
    this.db = db;
    if (db) {
      this.loadFromDb();
    }
  }

  create(userId?: string): Session {
    const session: Session = {
      id: randomUUID(),
      state: 'idle',
      createdAt: Date.now(),
      updatedAt: Date.now(),
      messages: [],
      metadata: {
        userId,
        questionsAsked: 0,
        dimensions: [],
      },
    };
    this.sessions.set(session.id, session);

    if (this.db) {
      this.db.prepare(`
        INSERT INTO sessions (id, state, user_id, metadata, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
      `).run(
        session.id,
        session.state,
        userId ?? null,
        JSON.stringify(session.metadata),
        Math.floor(session.createdAt / 1000),
        Math.floor(session.updatedAt / 1000),
      );
    }

    return session;
  }

  get(id: string): Session | undefined {
    return this.sessions.get(id);
  }

  transition(id: string, newState: SessionState): void {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`Session ${id} not found`);

    const valid = this.validTransitions(session.state);
    if (!valid.includes(newState)) {
      throw new Error(`Invalid transition: ${session.state} → ${newState}`);
    }

    session.state = newState;
    session.updatedAt = Date.now();

    if (this.db) {
      this.db.prepare('UPDATE sessions SET state = ?, updated_at = ? WHERE id = ?')
        .run(newState, Math.floor(session.updatedAt / 1000), id);
    }
  }

  addMessage(id: string, message: Message): void {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`Session ${id} not found`);

    session.messages.push(message);
    session.updatedAt = Date.now();

    if (message.role === 'user') {
      session.metadata.questionsAsked++;
    }

    if (this.db) {
      this.db.prepare(`
        INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, created_at)
        VALUES (?, ?, ?, ?, ?, ?)
      `).run(
        id,
        message.role,
        message.content ?? null,
        message.toolCallId ?? null,
        message.toolCalls ? JSON.stringify(message.toolCalls) : null,
        Math.floor(Date.now() / 1000),
      );

      this.db.prepare('UPDATE sessions SET metadata = ?, updated_at = ? WHERE id = ?')
        .run(JSON.stringify(session.metadata), Math.floor(session.updatedAt / 1000), id);
    }
  }

  replaceMessages(id: string, messages: Message[]): void {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`Session ${id} not found`);

    session.messages = messages;
    session.updatedAt = Date.now();

    if (this.db) {
      const tx = this.db.transaction(() => {
        this.db!.prepare('DELETE FROM messages WHERE session_id = ?').run(id);
        const insert = this.db!.prepare(`
          INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, created_at)
          VALUES (?, ?, ?, ?, ?, ?)
        `);
        for (const msg of messages) {
          insert.run(
            id,
            msg.role,
            msg.content ?? null,
            msg.toolCallId ?? null,
            msg.toolCalls ? JSON.stringify(msg.toolCalls) : null,
            Math.floor(Date.now() / 1000),
          );
        }
        this.db!.prepare('UPDATE sessions SET updated_at = ? WHERE id = ?')
          .run(Math.floor(session.updatedAt / 1000), id);
      });
      tx();
    }
  }

  checkpoint(id: string): Checkpoint {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`Session ${id} not found`);

    const cp: Checkpoint = {
      id: randomUUID(),
      sessionId: id,
      createdAt: Date.now(),
      messageIndex: session.messages.length,
      state: session.state,
      metadata: { ...session.metadata },
    };

    const list = this.checkpoints.get(id) ?? [];
    list.push(cp);
    this.checkpoints.set(id, list);

    return cp;
  }

  rewind(sessionId: string, checkpointId: string): void {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);

    const list = this.checkpoints.get(sessionId) ?? [];
    const cp = list.find((c) => c.id === checkpointId);
    if (!cp) throw new Error(`Checkpoint ${checkpointId} not found`);

    session.messages = session.messages.slice(0, cp.messageIndex);
    session.state = cp.state;
    session.metadata = { ...cp.metadata };
    session.updatedAt = Date.now();
  }

  listActive(): Session[] {
    return Array.from(this.sessions.values()).filter(
      (s) => s.state === 'active' || s.state === 'paused',
    );
  }

  private validTransitions(current: SessionState): SessionState[] {
    const map: Record<SessionState, SessionState[]> = {
      idle: ['active'],
      active: ['paused', 'completed', 'error'],
      paused: ['active', 'completed'],
      completed: [],
      error: ['active'],
    };
    return map[current];
  }

  private loadFromDb(): void {
    if (!this.db) return;

    const rows = this.db.prepare('SELECT * FROM sessions').all() as Array<{
      id: string;
      state: string;
      user_id: string | null;
      metadata: string | null;
      created_at: number;
      updated_at: number;
    }>;

    for (const row of rows) {
      const metadata = row.metadata ? JSON.parse(row.metadata) : { questionsAsked: 0, dimensions: [] };
      const session: Session = {
        id: row.id,
        state: row.state as SessionState,
        createdAt: row.created_at * 1000,
        updatedAt: row.updated_at * 1000,
        messages: [],
        metadata: { ...metadata, userId: row.user_id ?? undefined },
      };

      const msgs = this.db!.prepare('SELECT * FROM messages WHERE session_id = ? ORDER BY id')
        .all(row.id) as Array<{
          role: string;
          content: string | null;
          tool_call_id: string | null;
          tool_calls: string | null;
        }>;

      session.messages = msgs.map((m) => ({
        role: m.role as Message['role'],
        content: m.content ?? undefined,
        toolCallId: m.tool_call_id ?? undefined,
        toolCalls: m.tool_calls ? JSON.parse(m.tool_calls) : undefined,
      }));

      this.sessions.set(session.id, session);
    }
  }
}
