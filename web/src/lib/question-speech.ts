export type QuestionSpeechPhase = 'idle' | 'loading' | 'speaking';
export type QuestionSpeechSource = 'mimo' | 'browser' | null;

export interface QuestionSpeechState {
  phase: QuestionSpeechPhase;
  source: QuestionSpeechSource;
}

interface QuestionAudio {
  preload: string;
  onplaying: ((this: GlobalEventHandlers, event: Event) => unknown) | null;
  onended: ((this: GlobalEventHandlers, event: Event) => unknown) | null;
  onerror: ((this: GlobalEventHandlers, event: Event) => unknown) | null;
  play(): Promise<void> | void;
  pause(): void;
  removeAttribute(name: string): void;
  load(): void;
}

interface QuestionUtterance {
  lang: string;
  rate: number;
  onstart: ((this: SpeechSynthesisUtterance, event: SpeechSynthesisEvent) => unknown) | null;
  onend: ((this: SpeechSynthesisUtterance, event: SpeechSynthesisEvent) => unknown) | null;
  onerror: ((this: SpeechSynthesisUtterance, event: SpeechSynthesisErrorEvent) => unknown) | null;
}

interface QuestionSpeechSynthesis {
  cancel(): void;
  speak(utterance: QuestionUtterance): void;
}

export interface QuestionSpeechDependencies {
  fetch(input: string, init: RequestInit): Promise<Response>;
  createAudio(src: string): QuestionAudio;
  createObjectURL(blob: Blob): string;
  revokeObjectURL(url: string): void;
  speechSynthesis: QuestionSpeechSynthesis | null;
  createUtterance: ((text: string) => QuestionUtterance) | null;
}

export type QuestionSpeechListener = (state: QuestionSpeechState) => void;

const IDLE_STATE: QuestionSpeechState = { phase: 'idle', source: null };

export class QuestionSpeechController {
  private generation = 0;
  private abortController: AbortController | null = null;
  private audio: QuestionAudio | null = null;
  private objectUrl: string | null = null;
  private disposed = false;
  private state: QuestionSpeechState = IDLE_STATE;

  constructor(
    private readonly dependencies: QuestionSpeechDependencies,
    private readonly listener: QuestionSpeechListener,
  ) {}

  get active(): boolean {
    return this.state.phase !== 'idle';
  }

  async speak(text: string): Promise<void> {
    const normalizedText = text.trim();
    this.cancel();
    if (this.disposed || !normalizedText) return;

    const generation = this.generation;
    const controller = new AbortController();
    this.abortController = controller;
    this.updateState({ phase: 'loading', source: 'mimo' });

    let blob: Blob;
    try {
      const response = await this.dependencies.fetch('/api/tts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: normalizedText, format: 'mp3' }),
        signal: controller.signal,
      });
      if (!response.ok) {
        throw new Error(`TTS request failed with status ${response.status}`);
      }
      blob = await response.blob();
      if (blob.size === 0) {
        throw new Error('TTS returned empty audio');
      }
    } catch (error) {
      if (!this.isCurrent(generation) || controller.signal.aborted) return;
      this.abortController = null;
      this.speakWithBrowserVoice(normalizedText, generation);
      return;
    }

    if (!this.isCurrent(generation) || controller.signal.aborted) return;
    this.abortController = null;

    let audio: QuestionAudio;
    try {
      this.objectUrl = this.dependencies.createObjectURL(blob);
      audio = this.dependencies.createAudio(this.objectUrl);
      audio.preload = 'auto';
      this.audio = audio;
    } catch {
      this.releaseAudio();
      this.speakWithBrowserVoice(normalizedText, generation);
      return;
    }

    audio.onplaying = () => {
      if (this.isCurrent(generation) && this.audio === audio) {
        this.updateState({ phase: 'speaking', source: 'mimo' });
      }
    };
    audio.onended = () => this.finishAudio(generation, audio);
    audio.onerror = () => this.handleAudioFailure(normalizedText, generation, audio);

    try {
      await audio.play();
    } catch {
      this.handleAudioFailure(normalizedText, generation, audio);
    }
  }

  cancel(): void {
    this.generation += 1;
    this.abortController?.abort();
    this.abortController = null;
    this.releaseAudio();
    try {
      this.dependencies.speechSynthesis?.cancel();
    } catch {}
    if (!this.disposed) this.updateState(IDLE_STATE);
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.generation += 1;
    this.abortController?.abort();
    this.abortController = null;
    this.releaseAudio();
    try {
      this.dependencies.speechSynthesis?.cancel();
    } catch {}
  }

  private speakWithBrowserVoice(text: string, generation: number): void {
    if (!this.isCurrent(generation)) return;
    const { speechSynthesis, createUtterance } = this.dependencies;
    if (!speechSynthesis || !createUtterance) {
      this.updateState(IDLE_STATE);
      return;
    }

    let utterance: QuestionUtterance;
    try {
      utterance = createUtterance(text);
      utterance.lang = 'zh-CN';
      utterance.rate = 0.92;
    } catch {
      this.updateState(IDLE_STATE);
      return;
    }
    utterance.onstart = () => {
      if (this.isCurrent(generation)) {
        this.updateState({ phase: 'speaking', source: 'browser' });
      }
    };
    const finish = () => {
      if (this.isCurrent(generation)) this.updateState(IDLE_STATE);
    };
    utterance.onend = finish;
    utterance.onerror = finish;
    this.updateState({ phase: 'loading', source: 'browser' });
    try {
      speechSynthesis.speak(utterance);
    } catch {
      this.updateState(IDLE_STATE);
    }
  }

  private finishAudio(generation: number, audio: QuestionAudio): void {
    if (!this.isCurrent(generation) || this.audio !== audio) return;
    this.releaseAudio();
    this.updateState(IDLE_STATE);
  }

  private handleAudioFailure(text: string, generation: number, audio: QuestionAudio): void {
    if (!this.isCurrent(generation) || this.audio !== audio) return;
    this.releaseAudio();
    this.speakWithBrowserVoice(text, generation);
  }

  private releaseAudio(): void {
    const audio = this.audio;
    this.audio = null;
    if (audio) {
      audio.onplaying = null;
      audio.onended = null;
      audio.onerror = null;
      try {
        audio.pause();
        audio.removeAttribute('src');
        audio.load();
      } catch {}
    }

    const objectUrl = this.objectUrl;
    this.objectUrl = null;
    if (objectUrl) {
      try {
        this.dependencies.revokeObjectURL(objectUrl);
      } catch {}
    }
  }

  private isCurrent(generation: number): boolean {
    return !this.disposed && this.generation === generation;
  }

  private updateState(state: QuestionSpeechState): void {
    this.state = state;
    this.listener(state);
  }
}

export function createBrowserQuestionSpeechController(listener: QuestionSpeechListener): QuestionSpeechController {
  const browserSpeechSynthesis = 'speechSynthesis' in window ? window.speechSynthesis : null;
  const createUtterance = typeof SpeechSynthesisUtterance === 'undefined'
    ? null
    : (text: string) => new SpeechSynthesisUtterance(text);

  return new QuestionSpeechController({
    fetch: (input, init) => window.fetch(input, init),
    createAudio: (src) => new Audio(src),
    createObjectURL: (blob) => URL.createObjectURL(blob),
    revokeObjectURL: (url) => URL.revokeObjectURL(url),
    speechSynthesis: browserSpeechSynthesis ? {
      cancel: () => browserSpeechSynthesis.cancel(),
      speak: (utterance) => browserSpeechSynthesis.speak(utterance as SpeechSynthesisUtterance),
    } : null,
    createUtterance,
  }, listener);
}
