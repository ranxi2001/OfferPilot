import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { createCanvas } from '@napi-rs/canvas';
import type { PDFPageProxy } from 'pdfjs-dist/types/src/display/api';

const pdfjsRoot = resolvePDFJSRoot();
export const pdfjsWorkerUrl = pathToFileURL(join(pdfjsRoot, 'legacy/build/pdf.worker.mjs')).href;

export const pdfjsAssetDirectories = {
  cMapUrl: assetDirectory('cmaps'),
  standardFontDataUrl: assetDirectory('standard_fonts'),
};

let pdfjsModule: Promise<typeof import('pdfjs-dist/legacy/build/pdf.mjs')> | null = null;

interface ExtractPDFOptions {
  renderPages?: boolean;
  maxRenderedPages?: number;
}

export async function extractPdfText(buffer: Uint8Array) {
  return extractPdfDocument(buffer);
}

export async function extractPdfDocument(buffer: Uint8Array, options: ExtractPDFOptions = {}) {
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
    const pages: string[] = [];
    const pageImages: string[] = [];
    const renderedPageLimit = Math.min(pdf.numPages, options.maxRenderedPages ?? 3);
    for (let index = 0; index < pdf.numPages; index++) {
      const page = await pdf.getPage(index + 1);
      const content = await page.getTextContent();
      pages.push(content.items
        .filter((item): item is typeof item & { str: string; hasEOL?: boolean } => 'str' in item)
        .map((item) => `${item.str}${item.hasEOL ? '\n' : ''}`)
        .join(''));

      if (options.renderPages && index < renderedPageLimit) {
        pageImages.push(await renderPageAsJPEG(page));
      }
    }
    return {
      totalPages: pdf.numPages,
      text: normalizePDFText(pages.join('\n\n')),
      pageImages,
    };
  } finally {
    await loadingTask.destroy();
  }
}

async function renderPageAsJPEG(page: PDFPageProxy): Promise<string> {
  const baseViewport = page.getViewport({ scale: 1 });
  const scale = Math.min(1.5, 1100 / baseViewport.width);
  const viewport = page.getViewport({ scale });
  const canvas = createCanvas(Math.ceil(viewport.width), Math.ceil(viewport.height));
  await page.render({
    canvas: canvas as unknown as HTMLCanvasElement,
    viewport,
    background: '#ffffff',
  }).promise;
  return canvas.toDataURL('image/jpeg', 0.82);
}

export function normalizePDFText(text: string): string {
  return text
    .replace(/[\t\f\v ]+/g, ' ')
    .replace(/ *\n */g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
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
