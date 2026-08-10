import { NextRequest, NextResponse } from 'next/server';
import { lookup } from 'node:dns/promises';
import { BlockList, isIP, type LookupFunction } from 'node:net';
import { Agent, fetch as undiciFetch } from 'undici';
import { MAX_URL_RESPONSE_BYTES, readJsonBody } from '@/lib/api-security';

const MAX_REDIRECTS = 3;
const REQUEST_TIMEOUT_MS = 10000;
const ALLOW_TUN_FAKE_IP = readBooleanEnv(
  'OFFERPILOT_ALLOW_TUN_FAKE_IP',
  process.env.NODE_ENV !== 'production',
);
const REQUEST_HEADERS = {
  'User-Agent': 'Mozilla/5.0 (compatible; OfferPilot/1.0; +https://offerpilot.dev)',
  'Accept': 'text/html,application/xhtml+xml,text/plain',
};
const NON_PUBLIC_ADDRESSES = createNonPublicAddressBlockList();

type FetchResponse = Awaited<ReturnType<typeof undiciFetch>>;

interface PinnedFetch {
  response: FetchResponse;
  dispatcher: Agent;
}

interface PinnedAddress {
  address: string;
  family: 4 | 6;
}

export const runtime = 'nodejs';

export async function POST(req: NextRequest) {
  try {
    const parsed = await readJsonBody<{ url?: string }>(req);
    if (parsed.response) return parsed.response;
    const { url } = parsed.data;

    if (!url?.trim()) {
      return NextResponse.json({ error: 'URL is required' }, { status: 400 });
    }

    let normalizedUrl = normalizeUrl(url);
    let activeFetch: PinnedFetch | null = null;

    try {
      for (let i = 0; i <= MAX_REDIRECTS; i++) {
        activeFetch = await fetchPinnedPublicUrl(normalizedUrl);
        const { response } = activeFetch;

        if (!isRedirect(response.status)) break;

        const location = response.headers.get('location');
        if (!location) break;
        if (i === MAX_REDIRECTS) {
          throw new Error(`URL redirected more than ${MAX_REDIRECTS} times`);
        }

        await releasePinnedFetch(activeFetch);
        activeFetch = null;
        normalizedUrl = normalizeUrl(new URL(location, normalizedUrl).toString());
      }

      if (!activeFetch) {
        return NextResponse.json({ error: 'Failed to fetch URL' }, { status: 400 });
      }

      if (!activeFetch.response.ok) {
        return NextResponse.json(
          { error: `Failed to fetch URL (${activeFetch.response.status})` },
          { status: 400 },
        );
      }

      const html = await readLimitedText(activeFetch.response, MAX_URL_RESPONSE_BYTES);
      const text = extractTextFromHtml(html);

      if (!text.trim()) {
        return NextResponse.json({ error: 'No text content found at URL' }, { status: 400 });
      }

      return NextResponse.json({ text, source: normalizedUrl });
    } finally {
      if (activeFetch) await releasePinnedFetch(activeFetch);
    }
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
      || msg.includes('did not resolve')
      || msg.includes('redirected more than')) {
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

async function fetchPinnedPublicUrl(raw: string): Promise<PinnedFetch> {
  const url = new URL(raw);
  const pinnedAddress = await resolvePublicAddress(url);
  const expectedHostname = stripIpv6Brackets(url.hostname).toLowerCase();
  const pinnedLookup: LookupFunction = (hostname, options, callback) => {
    if (stripIpv6Brackets(hostname).toLowerCase() !== expectedHostname) {
      const error = new Error('Pinned DNS lookup received an unexpected hostname') as NodeJS.ErrnoException;
      error.code = 'ENOTFOUND';
      callback(error, '');
      return;
    }

    if (options.all) {
      callback(null, [pinnedAddress]);
      return;
    }
    callback(null, pinnedAddress.address, pinnedAddress.family);
  };
  const dispatcher = new Agent({
    // Keep the URL hostname for Host/SNI while the socket lookup returns only this validated address.
    connect: { lookup: pinnedLookup },
    connections: 1,
    pipelining: 0,
  });

  try {
    const response = await undiciFetch(url, {
      headers: REQUEST_HEADERS,
      redirect: 'manual',
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
      dispatcher,
    });
    return { response, dispatcher };
  } catch (error) {
    await dispatcher.destroy().catch(() => {});
    throw error;
  }
}

async function resolvePublicAddress(url: URL): Promise<PinnedAddress> {
  const host = stripIpv6Brackets(url.hostname);
  const literalFamily = isIP(host);
  const addresses = literalFamily
    ? [{ address: host, family: literalFamily }]
    : await lookup(host, { all: true, verbatim: true });

  if (addresses.length === 0) {
    throw new Error('URL host did not resolve');
  }

  for (const item of addresses) {
    const developmentTunnelAddress = !literalFamily
      && ALLOW_TUN_FAKE_IP
      && isBenchmarkTunnelAddress(item.address);
    if (isPrivateAddress(item.address) && !developmentTunnelAddress) {
      throw new Error('Private, loopback, and link-local URLs are not allowed');
    }
  }

  const selected = addresses[0];
  const family = isIP(selected.address);
  if (family !== 4 && family !== 6) {
    throw new Error('URL host did not resolve to a valid IP address');
  }
  return { address: selected.address, family };
}

async function releasePinnedFetch(fetchResult: PinnedFetch): Promise<void> {
  const { response, dispatcher } = fetchResult;
  if (response.body && !response.bodyUsed) {
    await response.body.cancel().catch(() => {});
  }
  await dispatcher.destroy().catch(() => {});
}

function stripIpv6Brackets(hostname: string): string {
  return hostname.startsWith('[') && hostname.endsWith(']')
    ? hostname.slice(1, -1)
    : hostname;
}

function isRedirect(status: number): boolean {
  return status === 301 || status === 302 || status === 303 || status === 307 || status === 308;
}

function isPrivateAddress(address: string): boolean {
  const version = isIP(address);
  if (version === 4) return NON_PUBLIC_ADDRESSES.check(address, 'ipv4');
  if (version === 6) return NON_PUBLIC_ADDRESSES.check(address, 'ipv6');
  return true;
}

function isBenchmarkTunnelAddress(address: string): boolean {
  if (isIP(address) !== 4) return false;
  const [first, second] = address.split('.').map((part) => Number.parseInt(part, 10));
  return first === 198 && (second === 18 || second === 19);
}

function readBooleanEnv(name: string, fallback: boolean): boolean {
  const value = process.env[name]?.trim().toLowerCase();
  if (!value) return fallback;
  if (value === 'true' || value === '1') return true;
  if (value === 'false' || value === '0') return false;
  return fallback;
}

function createNonPublicAddressBlockList(): BlockList {
  const blockList = new BlockList();
  const ipv4Subnets: Array<[string, number]> = [
    ['0.0.0.0', 8],
    ['10.0.0.0', 8],
    ['100.64.0.0', 10],
    ['127.0.0.0', 8],
    ['169.254.0.0', 16],
    ['172.16.0.0', 12],
    ['192.0.0.0', 24],
    ['192.0.2.0', 24],
    ['192.88.99.0', 24],
    ['192.168.0.0', 16],
    ['198.18.0.0', 15],
    ['198.51.100.0', 24],
    ['203.0.113.0', 24],
    ['224.0.0.0', 4],
    ['240.0.0.0', 4],
  ];
  const ipv6Subnets: Array<[string, number]> = [
    ['::', 96],
    ['::ffff:0:0', 96],
    ['64:ff9b::', 96],
    ['64:ff9b:1::', 48],
    ['100::', 64],
    ['2001::', 23],
    ['2001:db8::', 32],
    ['2002::', 16],
    ['fc00::', 7],
    ['fe80::', 10],
    ['fec0::', 10],
    ['ff00::', 8],
  ];

  for (const [network, prefix] of ipv4Subnets) blockList.addSubnet(network, prefix, 'ipv4');
  for (const [network, prefix] of ipv6Subnets) blockList.addSubnet(network, prefix, 'ipv6');
  return blockList;
}

async function readLimitedText(response: FetchResponse, maxBytes: number): Promise<string> {
  const reader = response.body?.getReader();
  if (!reader) return '';

  const chunks: Uint8Array[] = [];
  let total = 0;
  let complete = false;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        complete = true;
        break;
      }
      if (!value) continue;
      total += value.byteLength;
      if (total > maxBytes) {
        throw new Error(`URL response exceeds ${maxBytes} bytes`);
      }
      chunks.push(value);
    }
  } finally {
    if (!complete) await reader.cancel().catch(() => {});
    reader.releaseLock();
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
