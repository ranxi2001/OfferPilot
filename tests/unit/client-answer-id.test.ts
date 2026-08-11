import { describe, expect, it, vi } from 'vitest';
import {
  CLIENT_ANSWER_STORAGE_KEY,
  clearClientAnswerDescriptor,
  createClientAnswerId,
  getOrCreateClientAnswerId,
  readClientAnswerDescriptor,
  type ClientAnswerStorage,
} from '../../web/src/lib/client-answer-id.js';

class MemoryStorage implements ClientAnswerStorage {
  private readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }
}

describe('client answer id', () => {
  it('prefers crypto.randomUUID when the browser provides it', () => {
    const randomUUID = vi.fn(() => '0198a878-e073-7dd5-a16b-8fb46c487a30');

    expect(createClientAnswerId({ randomUUID })).toBe('0198a878-e073-7dd5-a16b-8fb46c487a30');
    expect(randomUUID).toHaveBeenCalledOnce();
  });

  it('creates an RFC 4122 version 4 UUID with getRandomValues as a fallback', () => {
    const clientAnswerId = createClientAnswerId({
      getRandomValues(bytes) {
        bytes.forEach((_, index) => { bytes[index] = index; });
        return bytes;
      },
    });

    expect(clientAnswerId).toBe('00010203-0405-4607-8809-0a0b0c0d0e0f');
  });

  it('reuses the persisted id for the same interview and question after a refresh', () => {
    const storage = new MemoryStorage();
    const firstFactory = vi.fn(() => 'answer-1');
    const refreshedFactory = vi.fn(() => 'should-not-be-used');

    expect(getOrCreateClientAnswerId(storage, 'interview-1', 'question-1', firstFactory)).toBe('answer-1');
    expect(getOrCreateClientAnswerId(storage, 'interview-1', 'question-1', refreshedFactory)).toBe('answer-1');
    expect(firstFactory).toHaveBeenCalledOnce();
    expect(refreshedFactory).not.toHaveBeenCalled();
  });

  it('replaces the minimal descriptor only when the active question changes', () => {
    const storage = new MemoryStorage();
    getOrCreateClientAnswerId(storage, 'interview-1', 'question-1', () => 'answer-1');
    const nextId = getOrCreateClientAnswerId(storage, 'interview-1', 'question-2', () => 'answer-2');

    expect(nextId).toBe('answer-2');
    expect(readClientAnswerDescriptor(storage)).toEqual({
      interviewId: 'interview-1',
      questionId: 'question-2',
      clientAnswerId: 'answer-2',
    });
  });

  it('ignores malformed persisted data and clears the descriptor on reset', () => {
    const storage = new MemoryStorage();
    storage.setItem(CLIENT_ANSWER_STORAGE_KEY, '{broken');

    expect(getOrCreateClientAnswerId(storage, 'interview-1', 'question-1', () => 'answer-1')).toBe('answer-1');
    clearClientAnswerDescriptor(storage);
    expect(readClientAnswerDescriptor(storage)).toBeNull();
  });
});
