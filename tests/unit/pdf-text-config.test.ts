import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { normalizePDFText } from '../../web/src/lib/pdf-text';

describe('PDF text extraction contract', () => {
  it('pins the secure PDF.js runtime and configures its worker and CMap assets', () => {
    const manifest = JSON.parse(readFileSync(resolve('web/package.json'), 'utf8')) as {
      dependencies?: Record<string, string>;
    };
    const source = readFileSync(resolve('web/src/lib/pdf-text.ts'), 'utf8');

    expect(manifest.dependencies?.['pdfjs-dist']).toBe('6.2.108');
    expect(manifest.dependencies?.['@napi-rs/canvas']).toBe('1.0.8');
    expect(source).toContain("cMapPacked: true");
    expect(source).toContain("standardFontDataUrl: pdfjsAssetDirectories.standardFontDataUrl");
    expect(source).toContain("GlobalWorkerOptions.workerSrc = pdfjsWorkerUrl");
    expect(source).toContain("await loadingTask.destroy()");
    expect(source).toContain("page.render({");
    expect(source).toContain("canvas.toDataURL('image/jpeg', 0.82)");
  });

  it('preserves section and bullet line breaks while normalizing inline spaces', () => {
    expect(normalizePDFText('教育背景  \n  中国科学院大学\n\n\n•  项目经历')).toBe(
      '教育背景\n中国科学院大学\n\n• 项目经历',
    );
  });
});
