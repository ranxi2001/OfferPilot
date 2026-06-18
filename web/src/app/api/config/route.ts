import { NextRequest, NextResponse } from 'next/server';
import { readFileSync, writeFileSync, existsSync } from 'fs';
import { resolve } from 'path';
import yaml from 'js-yaml';

interface ModelEntry {
  name: string;
  provider: string;
  model: string;
  env_key: string;
  base_url?: string;
}

interface ModelsConfig {
  text?: ModelEntry[];
  tts?: ModelEntry[];
  multimodal?: ModelEntry[];
}

const PROJECT_ROOT = resolve(process.cwd(), '..');
const ENV_PATH = resolve(PROJECT_ROOT, '.env');
const MODELS_YML_PATH = resolve(PROJECT_ROOT, 'models.yml');

function loadEnv(): Record<string, string> {
  const env: Record<string, string> = {};
  if (!existsSync(ENV_PATH)) return env;
  const lines = readFileSync(ENV_PATH, 'utf-8').split('\n');
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const eqIdx = trimmed.indexOf('=');
    if (eqIdx === -1) continue;
    const key = trimmed.slice(0, eqIdx).trim();
    const val = trimmed.slice(eqIdx + 1).trim();
    env[key] = val;
  }
  return env;
}

function resolveEnvRef(value: string | undefined, env: Record<string, string>): string {
  if (!value) return '';
  return value.replace(/\$\{(\w+)\}/g, (_, key) => env[key] ?? process.env[key] ?? '');
}

function loadModelsConfig(): ModelsConfig {
  try {
    const raw = readFileSync(MODELS_YML_PATH, 'utf-8');
    return yaml.load(raw) as ModelsConfig;
  } catch {
    return { text: [], tts: [], multimodal: [] };
  }
}

export async function GET() {
  const config = loadModelsConfig();
  const env = loadEnv();
  const merged = { ...env };
  for (const [k, v] of Object.entries(process.env)) {
    if (v && !merged[k]) merged[k] = v;
  }

  const toResponse = (entries: ModelEntry[] | undefined) =>
    (entries ?? []).map((e) => ({
      name: e.name,
      provider: e.provider,
      model: resolveEnvRef(e.model, merged),
      base_url: resolveEnvRef(e.base_url, merged),
      env_key: e.env_key,
      model_env_key: e.model?.match(/\$\{(\w+)\}/)?.[1] ?? null,
      base_url_env_key: e.base_url?.match(/\$\{(\w+)\}/)?.[1] ?? null,
      available: !!(merged[e.env_key] && merged[e.env_key] !== `sk-ant-...` && merged[e.env_key] !== `sk-...`),
    }));

  const envVars: Record<string, string> = {};
  const allKeys = new Set<string>();
  for (const group of [config.text, config.tts, config.multimodal]) {
    for (const entry of group ?? []) {
      allKeys.add(entry.env_key);
      const modelRef = entry.model?.match(/\$\{(\w+)\}/)?.[1];
      if (modelRef) allKeys.add(modelRef);
      const baseRef = entry.base_url?.match(/\$\{(\w+)\}/)?.[1];
      if (baseRef) allKeys.add(baseRef);
    }
  }
  for (const key of allKeys) {
    envVars[key] = merged[key] ?? '';
  }

  return NextResponse.json({
    text: toResponse(config.text),
    tts: toResponse(config.tts),
    multimodal: toResponse(config.multimodal),
    envVars,
  });
}

export async function POST(req: NextRequest) {
  try {
    const { envVars } = (await req.json()) as { envVars: Record<string, string> };

    if (!envVars || typeof envVars !== 'object') {
      return NextResponse.json({ error: 'envVars object is required' }, { status: 400 });
    }

    const existing = loadEnv();
    const merged = { ...existing, ...envVars };

    const lines: string[] = [
      '# OfferPilot 模型配置 (由弹窗自动生成)',
      '# 手动编辑也会保留',
      '',
    ];

    const groups = [
      { label: '文本模型', keys: ['ANTHROPIC_API_KEY', 'ANTHROPIC_BASE_URL', 'ANTHROPIC_MODEL', 'OPENAI_API_KEY', 'OPENAI_BASE_URL', 'OPENAI_MODEL', 'DEEPSEEK_API_KEY', 'DEEPSEEK_BASE_URL', 'DEEPSEEK_MODEL'] },
      { label: 'TTS 语音合成', keys: ['MIMO_API_KEY', 'MIMO_BASE_URL', 'MIMO_TTS_MODEL', 'OPENAI_TTS_MODEL'] },
      { label: '语音识别', keys: ['MIMO_ASR_MODEL', 'OPENAI_ASR_MODEL'] },
    ];

    const written = new Set<string>();
    for (const group of groups) {
      lines.push(`# --- ${group.label} ---`);
      for (const key of group.keys) {
        if (merged[key]) {
          lines.push(`${key}=${merged[key]}`);
          written.add(key);
        }
      }
      lines.push('');
    }

    for (const [key, val] of Object.entries(merged)) {
      if (!written.has(key) && val) {
        lines.push(`${key}=${val}`);
      }
    }

    writeFileSync(ENV_PATH, lines.join('\n') + '\n', 'utf-8');

    return NextResponse.json({ ok: true });
  } catch (err) {
    return NextResponse.json({ error: (err as Error).message }, { status: 500 });
  }
}
