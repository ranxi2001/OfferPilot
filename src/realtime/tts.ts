import type { TTSConfig } from './types.js';

export interface TTSOutput {
  text: string;
  audioUrl?: string;
  ssml?: string;
}

export class TTSEngine {
  private config: TTSConfig;

  constructor(config?: Partial<TTSConfig>) {
    this.config = {
      provider: config?.provider ?? 'browser',
      voice: config?.voice ?? 'zh-CN-XiaoxiaoNeural',
      speed: config?.speed ?? 1.0,
      language: config?.language ?? 'zh-CN',
    };
  }

  async synthesize(text: string): Promise<TTSOutput> {
    switch (this.config.provider) {
      case 'edge-tts':
        return this.edgeTTS(text);
      case 'openai-tts':
        return this.openaiTTS(text);
      case 'browser':
      default:
        return this.browserTTS(text);
    }
  }

  private async browserTTS(text: string): Promise<TTSOutput> {
    // Browser Web Speech API — handled on client side
    // Server just returns the text and SSML markup
    return {
      text,
      ssml: this.buildSSML(text),
    };
  }

  private async edgeTTS(text: string): Promise<TTSOutput> {
    // Edge TTS via command line (edge-tts Python package)
    // In production, spawn process: edge-tts --voice zh-CN-XiaoxiaoNeural --text "..." --write-media output.mp3
    return {
      text,
      ssml: this.buildSSML(text),
      audioUrl: `/api/tts/edge?text=${encodeURIComponent(text)}&voice=${this.config.voice}`,
    };
  }

  private async openaiTTS(text: string): Promise<TTSOutput> {
    // OpenAI TTS API
    // In production: POST https://api.openai.com/v1/audio/speech
    return {
      text,
      audioUrl: `/api/tts/openai?text=${encodeURIComponent(text)}`,
    };
  }

  private buildSSML(text: string): string {
    return `<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="${this.config.language}">
  <voice name="${this.config.voice}">
    <prosody rate="${this.config.speed}">
      ${text}
    </prosody>
  </voice>
</speak>`;
  }
}
