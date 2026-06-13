export interface RealtimeSession {
  id: string;
  state: 'idle' | 'questioning' | 'answering' | 'analyzing' | 'speaking';
  currentQuestion: string | null;
  questionsAsked: number;
  totalQuestions: number;
  transcript: TranscriptEntry[];
  defects: DefectEntry[];
}

export interface TranscriptEntry {
  speaker: 'interviewer' | 'candidate';
  text: string;
  timestamp: number;
  duration?: number;
}

export interface DefectEntry {
  id: string;
  type: DefectType;
  severity: 'minor' | 'moderate' | 'critical';
  description: string;
  timestamp: number;
  suggestion: string;
}

export type DefectType =
  | 'too_vague'
  | 'missing_example'
  | 'no_structure'
  | 'factual_error'
  | 'too_short'
  | 'off_topic'
  | 'no_depth'
  | 'filler_words'
  | 'hesitation';

export interface TTSConfig {
  provider: 'browser' | 'edge-tts' | 'openai-tts';
  voice?: string;
  speed?: number;
  language?: string;
}

export interface STTConfig {
  provider: 'browser' | 'whisper' | 'funasr';
  language?: string;
  realtime?: boolean;
}
