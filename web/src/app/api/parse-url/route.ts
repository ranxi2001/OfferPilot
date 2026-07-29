import { NextRequest, NextResponse } from 'next/server';
import { lookup } from 'node:dns/promises';
import { isIP } from 'node:net';
import { MAX_URL_RESPONSE_BYTES, readJsonBody } from '@/lib/api-security';

const MAX_REDIRECTS = 3;

export async function POST(req: NextRequest) {
  try {
    const parsed = await readJsonBody<{ url?: string }>(req);
    if (parsed.response) return parsed.response;
    const { url } = parsed.data;

    if (!url?.trim()) {
      return NextResponse.json({ error: 'URL is required' }, { status: 400 });
    }

    let normalizedUrl = normalizeUrl(url);
    let res: Response | null = null;

    for (let i = 0; i <= MAX_REDIRECTS; i++) {
      await assertPublicHttpUrl(normalizedUrl);
      res = await fetch(normalizedUrl, {
        headers: {
          'User-Agent': 'Mozilla/5.0 (compatible; OfferPilot/1.0; +https://offerpilot.dev)',
          'Accept': 'text/html,application/xhtml+xml,text/plain',
        },
        redirect: 'manual',
        signal: AbortSignal.timeout(10000),
      });

      if (![301, 302, 303, 307, 308].includes(res.status)) break;

      const location = res.headers.get('location');
      if (!location) break;
      normalizedUrl = new URL(location, normalizedUrl).toString();
    }

    if (!res) {
      return NextResponse.json({ error: 'Failed to fetch URL' }, { status: 400 });
    }

    if (!res.ok) {
      return NextResponse.json(
        { error: `Failed to fetch URL (${res.status})` },
        { status: 400 },
      );
    }

    const html = await readLimitedText(res, MAX_URL_RESPONSE_BYTES);
    const text = extractTextFromHtml(html);

    if (!text.trim()) {
      return NextResponse.json({ error: 'No text content found at URL' }, { status: 400 });
    }

    return NextResponse.json({ text, source: normalizedUrl });
  } catch (err) {
    const msg = (err as Error).message;
    if (msg.includes('timeout') || msg.includes('aborted')) {
      return NextResponse.json({ error: 'URL request timed out (10s)' }, { status: 408 });
    }
    if (msg.includes('exceeds')) {
      return NextResponse.json({ error: msg }, { status: 413 });
    }
    if (msg.includes('not allowed')
      || msg.includes('not supported')
      || msg.includes('Only http')
      || msg.includes('credentials')
      || msg.includes('did not resolve')) {
      return NextResponse.json({ error: msg }, { status: 400 });
    }
    return NextResponse.json({ error: `URL fetch failed: ${msg}` }, { status: 500 });
  }
}

function normalizeUrl(raw: string): string {
  const withProtocol = /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(raw.trim())
    ? raw.trim()
    : `https://${raw.trim()}`;
  const parsed = new URL(withProtocol);
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('Only http and https URLs are supported');
  }
  if (parsed.username || parsed.password) {
    throw new Error('URLs with embedded credentials are not supported');
  }
  return parsed.toString();
}

async function assertPublicHttpUrl(raw: string): Promise<void> {
  const url = new URL(raw);
  const host = url.hostname;
  const addresses = isIP(host)
    ? [{ address: host }]
    : await lookup(host, { all: true, verbatim: true });

  if (addresses.length === 0) {
    throw new Error('URL host did not resolve');
  }

  for (const item of addresses) {
    if (isPrivateAddress(item.address)) {
      throw new Error('Private, loopback, and link-local URLs are not allowed');
    }
  }
}

function isPrivateAddress(address: string): boolean {
  const version = isIP(address);
  if (version === 4) {
    const parts = address.split('.').map((p) => parseInt(p, 10));
    const [a, b] = parts;
    return a === 0
      || a === 10
      || a === 127
      || (a === 100 && b >= 64 && b <= 127)
      || (a === 169 && b === 254)
      || (a === 172 && b >= 16 && b <= 31)
      || (a === 192 && b === 168)
      || a >= 224;
  }

  if (version === 6) {
    const lower = address.toLowerCase();
    return lower === '::1'
      || lower === '::'
      || lower.startsWith('fc')
      || lower.startsWith('fd')
      || lower.startsWith('fe8')
      || lower.startsWith('fe9')
      || lower.startsWith('fea')
      || lower.startsWith('feb');
  }

  return true;
}

async function readLimitedText(response: Response, maxBytes: number): Promise<string> {
  const reader = response.body?.getReader();
  if (!reader) return '';

  const chunks: Uint8Array[] = [];
  let total = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    if (!value) continue;
    total += value.byteLength;
    if (total > maxBytes) {
      throw new Error(`URL response exceeds ${maxBytes} bytes`);
    }
    chunks.push(value);
  }

  const combined = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder().decode(combined);
}

function extractTextFromHtml(html: string): string {
  let text = html;
  text = text.replace(/<script[\s\S]*?<\/script>/gi, '');
  text = text.replace(/<style[\s\S]*?<\/style>/gi, '');
  text = text.replace(/<nav[\s\S]*?<\/nav>/gi, '');
  text = text.replace(/<footer[\s\S]*?<\/footer>/gi, '');
  text = text.replace(/<header[\s\S]*?<\/header>/gi, '');
  text = text.replace(/<!--[\s\S]*?-->/g, '');
  text = text.replace(/<br\s*\/?>/gi, '\n');
  text = text.replace(/<\/(?:p|div|h[1-6]|li|tr|section|article)>/gi, '\n');
  text = text.replace(/<[^>]+>/g, ' ');
  text = text.replace(/&nbsp;/g, ' ');
  text = text.replace(/&amp;/g, '&');
  text = text.replace(/&lt;/g, '<');
  text = text.replace(/&gt;/g, '>');
  text = text.replace(/&quot;/g, '"');
  text = text.replace(/&#\d+;/g, '');
  text = text.replace(/[ \t]+/g, ' ');
  text = text.replace(/\n[ \t]+/g, '\n');
  text = text.replace(/\n{3,}/g, '\n\n');
  return text.trim();
}
