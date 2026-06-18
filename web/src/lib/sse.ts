export interface SSEEvent {
  type: 'text_delta' | 'thinking_delta' | 'tool_call' | 'tool_result' | 'error' | 'done';
  content?: string;
  name?: string;
  message?: string;
  result?: string;
}

export async function* readSSE(response: Response): AsyncIterable<SSEEvent> {
  if (!response.body) return;

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      const data = line.slice(6);

      if (data === '[DONE]') {
        yield { type: 'done' };
        return;
      }

      try {
        yield JSON.parse(data) as SSEEvent;
      } catch {}
    }
  }
}
