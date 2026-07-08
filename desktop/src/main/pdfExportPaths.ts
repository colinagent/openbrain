import * as path from 'path';

function normalizeOptionalString(value: string | null | undefined): string {
  return typeof value === 'string' ? value.trim() : '';
}

export function replaceMarkdownLikeExtensionWithPdf(fileName: string): string {
  const trimmed = normalizeOptionalString(fileName);
  if (!trimmed) {
    return 'Untitled.pdf';
  }
  const ext = path.extname(trimmed);
  if (ext && ['.md', '.markdown'].includes(ext.toLowerCase())) {
    return `${trimmed.slice(0, -ext.length)}.pdf`;
  }
  if (ext.toLowerCase() === '.pdf') {
    return trimmed;
  }
  return `${trimmed}.pdf`;
}

export function buildMarkdownPdfDefaultPath(input: {
  sourcePath?: string | null;
  currentDir?: string | null;
}): string {
  const sourcePath = normalizeOptionalString(input.sourcePath);
  if (sourcePath) {
    const dir = path.dirname(sourcePath);
    const fileName = replaceMarkdownLikeExtensionWithPdf(path.basename(sourcePath));
    return path.join(dir, fileName);
  }

  const currentDir = normalizeOptionalString(input.currentDir);
  if (currentDir) {
    return path.join(currentDir, 'Untitled.pdf');
  }

  return 'Untitled.pdf';
}
