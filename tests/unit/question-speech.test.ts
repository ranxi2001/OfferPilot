import { describe, expect, it, vi } from 'vitest';
import {
  QuestionSpeechController,
  type QuestionSpeechDependencies,
  type QuestionSpeechState,
} from '../../web/src/lib/question-speech.js';

interface FakeAudio {
  src: string;
  preload: string;
  onplaying: (() => void) | null;
  onended: (() => void) | null;
  onerror: (() => void) | null;
  play: ReturnType<typeof vi.fn>;
  pause: ReturnType<typeof vi.fn>;
  removeAttribute: ReturnType<typeof vi.fn>;
  load: ReturnType<typeof vi.fn>;
}

interface FakeUtterance {
  text: string;
  lang: string;
  rate: number;
  onstart: (() => void) | null;
  onend: (() => void) | null;
  onerror: (() => void) | null;
}

function createHarness(
  fetchMock: QuestionSpeechDependencies['fetch'],
  playAudio: (audio: FakeAudio) => Promise<void> | void = async () => {},
) {
  const states: QuestionSpeechState[] = [];
  const audios: FakeAudio[] = [];
  const utterances: FakeUtterance[] = [];
  const revokeObjectURL = vi.fn();
  const speechSynthesis = {
    cancel: vi.fn(),
    speak: vi.fn(() => {}),
  };
  const dependencies: QuestionSpeechDependencies = {
    fetch: fetchMock,
    createAudio: (src) => {
      const audio: FakeAudio = {
        src,
        preload: '',
        onplaying: null,
        onended: null,
        onerror: null,
        play: vi.fn(() => playAudio(audio)),
        pause: vi.fn(),
        removeAttribute: vi.fn(),
        load: vi.fn(),
      };
      audios.push(audio);
      return audio;
    },
    createObjectURL: vi.fn(() => `blob:question-${audios.length + 1}`),
    revokeObjectURL,
    speechSynthesis,
    createUtterance: (text) => {
      const utterance: FakeUtterance = {
        text,
        lang: '',
        rate: 1,
        onstart: null,
        onend: null,
        onerror: null,
      };
      utterances.push(utterance);
      return utterance;
    },
  };
  const controller = new QuestionSpeechController(dependencies, (state) => states.push(state));
  return { controller, states, audios, utterances, dependencies, revokeObjectURL, speechSynthesis };
}

describe('question speech controller', () => {
  it('requests MiMo audio and plays the returned Blob without invoking browser speech', async () => {
    const fetchMock = vi.fn(async () => new Response(new Blob(['mimo-audio'], { type: 'audio/mpeg' })));
    const harness = createHarness(fetchMock);

    await harness.controller.speak('  请介绍这个项目  ');

    expect(fetchMock).toHaveBeenCalledWith('/api/tts', expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: '请介绍这个项目', format: 'mp3' }),
      signal: expect.any(AbortSignal),
    }));
    expect(harness.audios).toHaveLength(1);
    expect(harness.audios[0].src).toBe('blob:question-1');
    expect(harness.audios[0].preload).toBe('auto');
    expect(harness.audios[0].play).toHaveBeenCalledOnce();
    expect(harness.speechSynthesis.speak).not.toHaveBeenCalled();

    harness.audios[0].onplaying?.();
    expect(harness.states.at(-1)).toEqual({ phase: 'speaking', source: 'mimo' });
    harness.audios[0].onended?.();
    expect(harness.states.at(-1)).toEqual({ phase: 'idle', source: null });
    expect(harness.revokeObjectURL).toHaveBeenCalledWith('blob:question-1');
  });

  it('uses browser speech only after the MiMo request fails', async () => {
    const fetchMock = vi.fn(async () => new Response('unavailable', { status: 502 }));
    const harness = createHarness(fetchMock);

    await harness.controller.speak('解释一下索引失效');

    expect(harness.audios).toHaveLength(0);
    expect(harness.speechSynthesis.speak).toHaveBeenCalledOnce();
    expect(harness.utterances).toHaveLength(1);
    expect(harness.utterances[0]).toMatchObject({
      text: '解释一下索引失效',
      lang: 'zh-CN',
      rate: 0.92,
    });
    expect(harness.states.at(-1)).toEqual({ phase: 'loading', source: 'browser' });

    harness.utterances[0].onstart?.();
    expect(harness.states.at(-1)).toEqual({ phase: 'speaking', source: 'browser' });
    harness.utterances[0].onend?.();
    expect(harness.states.at(-1)).toEqual({ phase: 'idle', source: null });
  });

  it('falls back once and releases the Blob URL when MiMo audio cannot play', async () => {
    const fetchMock = vi.fn(async () => new Response(new Blob(['invalid-audio'], { type: 'audio/mpeg' })));
    const harness = createHarness(fetchMock, async (audio) => {
      audio.onerror?.();
      throw new DOMException('unsupported audio', 'NotSupportedError');
    });

    await harness.controller.speak('说明缓存击穿和缓存雪崩的区别');

    expect(harness.audios).toHaveLength(1);
    expect(harness.audios[0].pause).toHaveBeenCalledOnce();
    expect(harness.revokeObjectURL).toHaveBeenCalledOnce();
    expect(harness.speechSynthesis.speak).toHaveBeenCalledOnce();
    expect(harness.states.at(-1)).toEqual({ phase: 'loading', source: 'browser' });

    expect(harness.speechSynthesis.speak).toHaveBeenCalledOnce();
  });

  it('aborts an older request and ignores its late response', async () => {
    let resolveFirst: ((response: Response) => void) | undefined;
    let firstSignal: AbortSignal | undefined;
    const fetchMock = vi.fn()
      .mockImplementationOnce((_input: string, init: RequestInit) => {
        firstSignal = init.signal as AbortSignal;
        return new Promise<Response>((resolve) => {
          resolveFirst = resolve;
        });
      })
      .mockResolvedValueOnce(new Response(new Blob(['second'], { type: 'audio/mpeg' })));
    const harness = createHarness(fetchMock);

    const firstPlayback = harness.controller.speak('第一题');
    const secondPlayback = harness.controller.speak('第二题');
    await secondPlayback;

    expect(firstSignal?.aborted).toBe(true);
    expect(harness.audios).toHaveLength(1);
    expect(harness.audios[0].src).toBe('blob:question-1');

    resolveFirst?.(new Response(new Blob(['first'], { type: 'audio/mpeg' })));
    await firstPlayback;
    expect(harness.audios).toHaveLength(1);

    harness.controller.cancel();
    expect(harness.audios[0].pause).toHaveBeenCalledOnce();
    expect(harness.audios[0].removeAttribute).toHaveBeenCalledWith('src');
    expect(harness.revokeObjectURL).toHaveBeenCalledWith('blob:question-1');
    expect(harness.states.at(-1)).toEqual({ phase: 'idle', source: null });
  });

  it('disposes an in-flight request without falling back or updating state later', async () => {
    let rejectRequest: ((error: Error) => void) | undefined;
    let requestSignal: AbortSignal | undefined;
    const fetchMock = vi.fn((_input: string, init: RequestInit) => {
      requestSignal = init.signal as AbortSignal;
      return new Promise<Response>((_resolve, reject) => {
        rejectRequest = reject;
      });
    });
    const harness = createHarness(fetchMock);
    const playback = harness.controller.speak('不会继续播放');
    const stateCount = harness.states.length;

    harness.controller.dispose();
    expect(requestSignal?.aborted).toBe(true);
    rejectRequest?.(new DOMException('aborted', 'AbortError'));
    await playback;

    expect(harness.states).toHaveLength(stateCount);
    expect(harness.speechSynthesis.speak).not.toHaveBeenCalled();
  });
});
