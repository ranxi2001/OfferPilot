import { NextRequest, NextResponse } from 'next/server';
import { extractText } from 'unpdf';
import mammoth from 'mammoth';

export async function POST(req: NextRequest) {
  try {
    const formData = await req.formData();
    const file = formData.get('file') as File | null;

    if (!file) {
      return NextResponse.json({ error: 'No file provided' }, { status: 400 });
    }

    const name = file.name.toLowerCase();
    const buffer = new Uint8Array(await file.arrayBuffer());

    if (name.endsWith('.pdf')) {
      const { text, totalPages } = await extractText(buffer, { mergePages: true });
      return NextResponse.json({ text, pages: totalPages, format: 'pdf' });
    }

    if (name.endsWith('.docx') || name.endsWith('.doc')) {
      const result = await mammoth.extractRawText({ buffer: Buffer.from(buffer) });
      return NextResponse.json({ text: result.value, pages: null, format: 'docx' });
    }

    if (name.endsWith('.tex')) {
      const raw = new TextDecoder().decode(buffer);
      const text = stripLatex(raw);
      return NextResponse.json({ text, pages: null, format: 'tex' });
    }

    if (name.endsWith('.txt') || name.endsWith('.md')) {
      const text = new TextDecoder().decode(buffer);
      return NextResponse.json({ text, pages: null, format: name.endsWith('.md') ? 'md' : 'txt' });
    }

    return NextResponse.json({ error: 'Unsupported file format' }, { status: 400 });
  } catch (err) {
    return NextResponse.json(
      { error: `File parsing failed: ${(err as Error).message}` },
      { status: 500 },
    );
  }
}

function stripLatex(tex: string): string {
  let text = tex;
  text = text.replace(/%.*/g, '');
  text = text.replace(/\\begin\{document\}/, '').replace(/\\end\{document\}/, '');
  text = text.replace(/\\(?:documentclass|usepackage|pagestyle|geometry|setlength|renewcommand|newcommand)\{[^}]*\}(?:\[[^\]]*\])?(?:\{[^}]*\})*/g, '');
  text = text.replace(/\\(?:section|subsection|subsubsection|textbf|textit|emph|underline|href)\{([^}]*)\}/g, '$1');
  text = text.replace(/\\(?:begin|end)\{[^}]*\}/g, '');
  text = text.replace(/\\item\s*/g, '- ');
  text = text.replace(/\\[a-zA-Z]+\*?(?:\[[^\]]*\])?(?:\{([^}]*)\})?/g, '$1');
  text = text.replace(/[{}]/g, '');
  text = text.replace(/\n{3,}/g, '\n\n');
  return text.trim();
}
