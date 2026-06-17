export interface TranscribeAudioInput {
  audio: Buffer;
  fileName?: string;
  contentType?: string;
}

export interface TranscribeAudioOutput {
  text: string;
}

export interface SynthesizeSpeechInput {
  text: string;
  voice?: string;
  format?: string;
}

export interface SynthesizeSpeechOutput {
  audio: Buffer;
  contentType: string;
}

const DEFAULT_BASE_URL = 'https://api.xiaomimimo.com/v1';
const DEFAULT_ASR_MODEL = 'mimo-v2.5-asr';
const DEFAULT_TTS_MODEL = 'mimo-v2.5-tts';
const DEFAULT_TTS_VOICE = 'alloy';

function getConfig() {
  const apiKey = process.env.MIMO_API_KEY;
  if (!apiKey) {
    throw new Error('MIMO_API_KEY is not configured');
  }

  return {
    apiKey,
    baseUrl: (process.env.MIMO_BASE_URL ?? DEFAULT_BASE_URL).replace(/\/+$/, ''),
    asrModel: process.env.MIMO_ASR_MODEL ?? DEFAULT_ASR_MODEL,
    ttsModel: process.env.MIMO_TTS_MODEL ?? DEFAULT_TTS_MODEL,
    ttsVoice: process.env.MIMO_TTS_VOICE ?? DEFAULT_TTS_VOICE,
  };
}

export async function transcribeAudio(input: TranscribeAudioInput): Promise<TranscribeAudioOutput> {
  const config = getConfig();
  const fileName = input.fileName || 'answer.webm';
  const contentType = input.contentType || 'audio/webm';

  if (!isSupportedAsrAudio(contentType, fileName)) {
    throw new Error('Mimo ASR only supports wav or mp3 audio. Please record again or upload a .wav/.mp3 file.');
  }

  const dataUrl = `data:${normalizeAudioMime(contentType, fileName)};base64,${input.audio.toString('base64')}`;

  const response = await fetch(`${config.baseUrl}/chat/completions`, {
    method: 'POST',
    headers: {
      'api-key': config.apiKey,
      Authorization: `Bearer ${config.apiKey}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: config.asrModel,
      messages: [
        {
          role: 'user',
          content: [
            {
              type: 'input_audio',
              input_audio: {
                data: dataUrl,
              },
            },
          ],
        },
      ],
      asr_options: {
        language: process.env.MIMO_ASR_LANGUAGE ?? 'auto',
      },
    }),
  });

  if (!response.ok) {
    throw new Error(`Mimo ASR failed: ${response.status} ${await response.text()}`);
  }

  const data = await response.json() as {
    text?: string;
    choices?: Array<{ message?: { content?: string | Array<{ text?: string }> } }>;
  };
  const content = data.text ?? data.choices?.[0]?.message?.content;
  const text = extractText(content);
  if (!text) {
    throw new Error('Mimo ASR returned empty transcript');
  }

  return { text };
}

export async function synthesizeSpeech(input: SynthesizeSpeechInput): Promise<SynthesizeSpeechOutput> {
  const config = getConfig();
  const format = input.format ?? 'wav';

  const response = await fetch(`${config.baseUrl}/chat/completions`, {
    method: 'POST',
    headers: {
      'api-key': config.apiKey,
      Authorization: `Bearer ${config.apiKey}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: config.ttsModel,
      messages: [
        {
          role: 'user',
          content: '用自然、清晰、适合中文面试反馈的语气朗读。',
        },
        {
          role: 'assistant',
          content: input.text,
        },
      ],
      audio: {
        format,
        voice: input.voice ?? config.ttsVoice,
      },
    }),
  });

  if (!response.ok) {
    throw new Error(`Mimo TTS failed: ${response.status} ${await response.text()}`);
  }

  const data = await response.json() as { choices?: Array<{ message?: { audio?: { data?: string } } }> };
  const audioBase64 = data.choices?.[0]?.message?.audio?.data;
  if (!audioBase64) {
    throw new Error('Mimo TTS returned empty audio');
  }

  return {
    audio: Buffer.from(audioBase64, 'base64'),
    contentType: format === 'wav' ? 'audio/wav' : `audio/${format}`,
  };
}

function isSupportedAsrAudio(contentType: string, fileName: string): boolean {
  const mime = contentType.toLowerCase().split(';')[0].trim();
  const name = fileName.toLowerCase();
  return mime === 'audio/wav' || mime === 'audio/x-wav' || mime === 'audio/mpeg' || mime === 'audio/mp3'
    || name.endsWith('.wav') || name.endsWith('.mp3');
}

function normalizeAudioMime(contentType: string, fileName: string): string {
  const mime = contentType.toLowerCase().split(';')[0].trim();
  if (mime === 'audio/x-wav' || fileName.toLowerCase().endsWith('.wav')) return 'audio/wav';
  if (mime === 'audio/mp3' || fileName.toLowerCase().endsWith('.mp3')) return 'audio/mpeg';
  return mime;
}

function extractText(content: string | Array<{ text?: string }> | undefined): string {
  if (typeof content === 'string') return content.trim();
  if (Array.isArray(content)) {
    return content.map((item) => item.text ?? '').join('').trim();
  }
  return '';
}
