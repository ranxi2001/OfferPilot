import { describe, expect, it } from 'vitest';
import {
  normalizeTranscribeError,
  transcribeUnavailableError,
} from '../../web/src/lib/transcribe-error.js';

describe('transcription proxy errors', () => {
  it('does not expose provider request details from a legacy error response', () => {
    const payload = normalizeTranscribeError(JSON.stringify({
      error: 'speech: MiMo ASR: request: Post "https://api.xiaomimimo.com/v1/chat/completions": EOF',
    }), 502);

    expect(payload).toEqual({
      error: '语音识别服务暂时不可用，请重试',
      retryable: true,
    });
    expect(JSON.stringify(payload)).not.toMatch(/MiMo|xiaomi|https?:|EOF/i);
  });

  it('extracts a safe structured error and its retryability metadata', () => {
    const payload = normalizeTranscribeError(JSON.stringify({
      error: { message: '录音格式不受支持', retryable: false },
    }), 400);

    expect(payload).toEqual({ error: '录音格式不受支持', retryable: false });
  });

  it('extracts a safe top-level message and retryability metadata', () => {
    const payload = normalizeTranscribeError(JSON.stringify({
      message: '请求过于频繁',
      retryable: true,
    }), 429);

    expect(payload).toEqual({ error: '请求过于频繁', retryable: true });
  });

  it('uses a stable status-aware message when the upstream body is not JSON', () => {
    expect(normalizeTranscribeError('<html>gateway failure</html>', 504)).toEqual({
      error: '语音识别超时，请重试',
      retryable: true,
    });
  });

  it('returns a stable retryable error when the backend cannot be reached', () => {
    expect(transcribeUnavailableError()).toEqual({
      error: '无法连接语音识别服务，请稍后重试',
      retryable: true,
    });
  });

  it('translates known internal validation errors without exposing implementation names', () => {
    expect(normalizeTranscribeError(JSON.stringify({
      error: 'speech: MiMo ASR supports only WAV or MP3 audio',
    }), 502)).toEqual({
      error: '当前录音格式无法识别，请重新录制',
      retryable: true,
    });
  });
});
