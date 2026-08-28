import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

describe('PDF text extraction contract', () => {
  it('pins the secure PDF.js runtime and configures its worker and CMap assets', () => {
    const manifest = JSON.parse(readFileSync(resolve('web/package.json'), 'utf8')) as {
      dependencies?: Record<string, string>;
    };
    const source = readFileSync(resolve('web/src/lib/pdf-text.ts'), 'utf8');

    expect(manifest.dependencies?.['pdfjs-dist']).toBe('6.2.108');
    expect(source).toContain("cMapPacked: true");
    expect(source).toContain("standardFontDataUrl: pdfjsAssetDirectories.standardFontDataUrl");
    expect(source).toContain("GlobalWorkerOptions.workerSrc = pdfjsWorkerUrl");
    expect(source).toContain("await loadingTask.destroy()");
  });
});
