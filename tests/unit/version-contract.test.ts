import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

function readJson(path: string): { version: string; packages?: Record<string, { version?: string }> } {
  return JSON.parse(readFileSync(resolve(process.cwd(), path), 'utf8'));
}

describe('release version contract', () => {
  it('keeps package manifests, lockfiles, and the Go runtime version aligned', () => {
    const rootPackage = readJson('package.json');
    const rootLock = readJson('package-lock.json');
    const webPackage = readJson('web/package.json');
    const webLock = readJson('web/package-lock.json');
    const goVersionSource = readFileSync(
      resolve(process.cwd(), 'backend/internal/config/version.go'),
      'utf8',
    );
    const cliSource = readFileSync(resolve(process.cwd(), 'src/index.ts'), 'utf8');
    const goVersion = goVersionSource.match(/(?:const|var)\s+Version\s*=\s*"([^"]+)"/)?.[1];

    expect(rootPackage.version).toBe('0.4.0');
    expect(webPackage.version).toBe(rootPackage.version);
    expect(rootLock.version).toBe(rootPackage.version);
    expect(rootLock.packages?.['']?.version).toBe(rootPackage.version);
    expect(webLock.version).toBe(rootPackage.version);
    expect(webLock.packages?.['']?.version).toBe(rootPackage.version);
    expect(goVersion).toBe(rootPackage.version);
    expect(cliSource).toContain("new URL('../package.json', import.meta.url)");
    expect(cliSource).not.toMatch(/\.version\(['"]\d+\.\d+\.\d+['"]\)/);
  });
});
