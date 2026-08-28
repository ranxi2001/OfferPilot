import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { pdfjsAssetDirectories, pdfjsWorkerUrl } from '../../web/src/lib/pdf-text';

describe('PDF text extraction assets', () => {
  it('ships packed CMaps and standard fonts required by the Node PDF.js build', () => {
    expect(pdfjsAssetDirectories.cMapUrl.endsWith('/')).toBe(true);
    expect(pdfjsAssetDirectories.standardFontDataUrl.endsWith('/')).toBe(true);
    expect(existsSync(join(pdfjsAssetDirectories.cMapUrl, 'Adobe-GB1-UCS2.bcmap'))).toBe(true);
    expect(existsSync(join(pdfjsAssetDirectories.standardFontDataUrl, 'LiberationSans-Regular.ttf'))).toBe(true);
    expect(existsSync(fileURLToPath(pdfjsWorkerUrl))).toBe(true);
  });
});
