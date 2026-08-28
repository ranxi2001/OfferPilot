import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const pdfjsRoot = resolvePDFJSRoot();
export const pdfjsWorkerUrl = pathToFileURL(join(pdfjsRoot, 'legacy/build/pdf.worker.mjs')).href;

export const pdfjsAssetDirectories = {
  cMapUrl: assetDirectory('cmaps'),
  standardFontDataUrl: assetDirectory('standard_fonts'),
};

let pdfjsModule: Promise<typeof import('pdfjs-dist/legacy/build/pdf.mjs')> | null = null;

export async function extractPdfText(buffer: Uint8Array) {
  pdfjsModule ??= loadPDFJS();
  const pdfjs = await pdfjsModule;
  const loadingTask = pdfjs.getDocument({
    data: buffer,
    cMapUrl: pdfjsAssetDirectories.cMapUrl,
    cMapPacked: true,
    standardFontDataUrl: pdfjsAssetDirectories.standardFontDataUrl,
    useSystemFonts: true,
    disableFontFace: true,
  });
  try {
    const pdf = await loadingTask.promise;
    const pages = await Promise.all(
      Array.from({ length: pdf.numPages }, async (_, index) => {
        const page = await pdf.getPage(index + 1);
        const content = await page.getTextContent();
        return content.items
          .filter((item): item is typeof item & { str: string; hasEOL?: boolean } => 'str' in item)
          .map((item) => `${item.str}${item.hasEOL ? '\n' : ''}`)
          .join('');
      }),
    );
    return {
      totalPages: pdf.numPages,
      text: pages.join('\n').replace(/\s+/g, ' ').trim(),
    };
  } finally {
    await loadingTask.destroy();
  }
}

async function loadPDFJS() {
  const pdfjs = await import('pdfjs-dist/legacy/build/pdf.mjs');
  pdfjs.GlobalWorkerOptions.workerSrc = pdfjsWorkerUrl;
  return pdfjs;
}

function assetDirectory(name: string): string {
  return `${join(pdfjsRoot, name).replaceAll('\\', '/')}/`;
}

function resolvePDFJSRoot(): string {
  const projectRoot = process.env.OFFERPILOT_PROJECT_ROOT;
  const candidates = [
    join(process.cwd(), 'node_modules/pdfjs-dist'),
    join(process.cwd(), 'web/node_modules/pdfjs-dist'),
    ...(projectRoot ? [join(projectRoot, 'web/node_modules/pdfjs-dist')] : []),
  ];
  const resolved = candidates.find((candidate) => existsSync(join(candidate, 'package.json')));
  if (!resolved) {
    throw new Error('pdfjs-dist runtime assets are missing');
  }
  return resolved;
}
