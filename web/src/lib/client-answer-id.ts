export const CLIENT_ANSWER_STORAGE_KEY = 'offerpilot.interview.client-answer.v1';

export interface ClientAnswerDescriptor {
  interviewId: string;
  questionId: string;
  clientAnswerId: string;
}

export interface ClientAnswerStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

interface ClientAnswerRandomSource {
  randomUUID?: () => string;
  getRandomValues?: (bytes: Uint8Array) => Uint8Array;
}

export function createClientAnswerId(
  randomSource: ClientAnswerRandomSource | undefined = globalThis.crypto,
): string {
  if (typeof randomSource?.randomUUID === 'function') {
    return randomSource.randomUUID();
  }

  const bytes = new Uint8Array(16);
  if (typeof randomSource?.getRandomValues === 'function') {
    randomSource.getRandomValues(bytes);
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }

  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0'));
  return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`;
}

export function parseClientAnswerDescriptor(value: string | null): ClientAnswerDescriptor | null {
  if (!value) return null;

  try {
    const candidate = JSON.parse(value) as Partial<ClientAnswerDescriptor>;
    if (
      typeof candidate.interviewId !== 'string'
      || !candidate.interviewId
      || typeof candidate.questionId !== 'string'
      || !candidate.questionId
      || typeof candidate.clientAnswerId !== 'string'
      || !candidate.clientAnswerId
    ) {
      return null;
    }
    return {
      interviewId: candidate.interviewId,
      questionId: candidate.questionId,
      clientAnswerId: candidate.clientAnswerId,
    };
  } catch {
    return null;
  }
}

export function readClientAnswerDescriptor(
  storage: ClientAnswerStorage | null | undefined,
): ClientAnswerDescriptor | null {
  if (!storage) return null;
  try {
    return parseClientAnswerDescriptor(storage.getItem(CLIENT_ANSWER_STORAGE_KEY));
  } catch {
    return null;
  }
}

export function getOrCreateClientAnswerId(
  storage: ClientAnswerStorage | null | undefined,
  interviewId: string,
  questionId: string,
  createId: () => string = createClientAnswerId,
): string {
  const stored = readClientAnswerDescriptor(storage);
  if (stored?.interviewId === interviewId && stored.questionId === questionId) {
    return stored.clientAnswerId;
  }

  const clientAnswerId = createId();
  try {
    storage?.setItem(CLIENT_ANSWER_STORAGE_KEY, JSON.stringify({
      interviewId,
      questionId,
      clientAnswerId,
    } satisfies ClientAnswerDescriptor));
  } catch {
    // Storage can be unavailable in private browsing; the component also keeps an in-memory copy.
  }
  return clientAnswerId;
}

export function clearClientAnswerDescriptor(storage: ClientAnswerStorage | null | undefined): void {
  try {
    storage?.removeItem(CLIENT_ANSWER_STORAGE_KEY);
  } catch {
    // Reset remains usable even when browser storage is blocked.
  }
}
