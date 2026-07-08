export type BookHighlightFormat = 'epub' | 'pdf';

export type BookHighlightNoteInput = {
  sourcePath: string;
  sourceTitle?: string | null;
  format: BookHighlightFormat;
  text: string;
  locator?: string | null;
  cfi?: string | null;
  page?: number | null;
  rects?: BookHighlightRectInput[] | null;
  createdAt?: Date;
};

export type BookHighlightRectInput = {
  top: number;
  left: number;
  width: number;
  height: number;
};

export type ParsedBookHighlightNote = {
  type: 'book-highlight';
  source: string;
  title: string;
  format: BookHighlightFormat;
  text: string;
  locator: string;
  cfi: string | null;
  page: number | null;
  rects: BookHighlightRectInput[];
  created: string;
};

export type BookHighlightNoteTarget = {
  sourcePath: string;
  format: BookHighlightFormat;
};

export type BookHighlightNoteRemovalTarget = BookHighlightNoteTarget & {
  cfi?: string | null;
  page?: number | null;
  rects?: BookHighlightRectInput[] | null;
};

export type RemoveBookHighlightNoteResult = {
  content: string;
  removed: number;
};

const NOTE_HEADING = '# Highlights';
const RECT_EPSILON = 0.00002;

function normalizePosixPath(path: string): string {
  const absolute = path.startsWith('/');
  const stack: string[] = [];
  for (const part of path.split('/')) {
    if (!part || part === '.') {
      continue;
    }
    if (part === '..') {
      stack.pop();
      continue;
    }
    stack.push(part);
  }
  return `${absolute ? '/' : ''}${stack.join('/')}`;
}

function dirnamePosix(path: string): string {
  if (!path) {
    return '';
  }
  const normalized = normalizePosixPath(path);
  if (normalized === '/') {
    return '/';
  }
  const index = normalized.lastIndexOf('/');
  if (index <= 0) {
    return normalized.startsWith('/') ? '/' : '';
  }
  return normalized.slice(0, index);
}

function relativePath(fromPath: string, toPath: string): string {
  const fromDir = dirnamePosix(fromPath);
  const normalizedTo = normalizePosixPath(toPath.trim());
  if (!fromDir || !normalizedTo.startsWith('/')) {
    return normalizedTo;
  }

  const fromParts = fromDir.split('/').filter(Boolean);
  const toParts = normalizedTo.split('/').filter(Boolean);
  while (fromParts.length > 0 && toParts.length > 0 && fromParts[0] === toParts[0]) {
    fromParts.shift();
    toParts.shift();
  }
  const up = fromParts.map(() => '..');
  const parts = [...up, ...toParts];
  return parts.length > 0 ? parts.join('/') : normalizedTo.split('/').pop() || normalizedTo;
}

export function getBookNotePath(sourcePath: string): string {
  const normalized = normalizePosixPath(sourcePath.trim());
  const dir = dirnamePosix(normalized);
  const fileName = normalized.split('/').pop() || 'Book';
  const dotIndex = fileName.lastIndexOf('.');
  const baseName = dotIndex > 0 ? fileName.slice(0, dotIndex) : fileName;
  return normalizePosixPath(`${dir ? `${dir}/` : ''}${baseName || 'Book'}.md`);
}

function normalizeHighlightText(text: string): string {
  return text
    .replace(/\s+/g, ' ')
    .trim();
}

function normalizeNoteBody(text: string): string {
  return normalizeHighlightText(text);
}

function normalizeMetaValue(value: string): string {
  return value.replace(/\r?\n/g, ' ').trim();
}

function sanitizeRect(rect: BookHighlightRectInput): BookHighlightRectInput | null {
  const top = Number(rect.top);
  const left = Number(rect.left);
  const width = Number(rect.width);
  const height = Number(rect.height);
  if (![top, left, width, height].every(Number.isFinite) || width <= 0 || height <= 0) {
    return null;
  }
  return {
    top: Number(top.toFixed(6)),
    left: Number(left.toFixed(6)),
    width: Number(width.toFixed(6)),
    height: Number(height.toFixed(6)),
  };
}

function sanitizeRects(rects: BookHighlightRectInput[] | null | undefined): BookHighlightRectInput[] {
  return (rects || [])
    .map(sanitizeRect)
    .filter((rect): rect is BookHighlightRectInput => Boolean(rect));
}

function fenceForContent(content: string): string {
  let width = 3;
  const matches = content.match(/`{3,}/g) || [];
  for (const match of matches) {
    width = Math.max(width, match.length + 1);
  }
  return '`'.repeat(width);
}

function serializeNoteMeta(key: string, value: string | number | null | undefined): string | null {
  if (value === null || value === undefined || value === '') {
    return null;
  }
  return `${key}: ${normalizeMetaValue(String(value))}`;
}

function parseMetaLines(lines: string[]): Record<string, string> {
  const meta: Record<string, string> = {};
  for (const line of lines) {
    const index = line.indexOf(':');
    if (index <= 0) {
      continue;
    }
    const key = line.slice(0, index).trim().toLowerCase();
    const value = line.slice(index + 1).trim();
    if (key) {
      meta[key] = value;
    }
  }
  return meta;
}

function parseRects(value: string | undefined): BookHighlightRectInput[] {
  if (!value) {
    return [];
  }
  try {
    const parsed = JSON.parse(value) as unknown;
    if (!Array.isArray(parsed)) {
      return [];
    }
    return sanitizeRects(parsed as BookHighlightRectInput[]);
  } catch {
    return [];
  }
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function sameNormalizedPath(left: string, right: string): boolean {
  return normalizePosixPath(left.trim()) === normalizePosixPath(right.trim());
}

function resolveNoteSourcePath(notePath: string, source: string): string {
  const trimmed = source.trim();
  if (!trimmed) {
    return '';
  }
  if (trimmed.startsWith('/') || trimmed.startsWith('~/')) {
    return normalizePosixPath(trimmed);
  }
  return normalizePosixPath(`${dirnamePosix(notePath)}/${trimmed}`);
}

function isValidHighlightForFormat(note: ParsedBookHighlightNote): boolean {
  if (note.format === 'epub') {
    return Boolean(note.cfi);
  }
  return note.page !== null && note.rects.length > 0;
}

function sameRect(left: BookHighlightRectInput, right: BookHighlightRectInput): boolean {
  return Math.abs(left.top - right.top) <= RECT_EPSILON
    && Math.abs(left.left - right.left) <= RECT_EPSILON
    && Math.abs(left.width - right.width) <= RECT_EPSILON
    && Math.abs(left.height - right.height) <= RECT_EPSILON;
}

function sameRects(left: BookHighlightRectInput[], right: BookHighlightRectInput[]): boolean {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((rect, index) => sameRect(rect, right[index]));
}

function isTargetSource(note: ParsedBookHighlightNote, notePath: string, target: BookHighlightNoteTarget): boolean {
  return note.format === target.format
    && sameNormalizedPath(resolveNoteSourcePath(notePath, note.source), target.sourcePath);
}

function isRemovalTarget(note: ParsedBookHighlightNote, notePath: string, target: BookHighlightNoteRemovalTarget): boolean {
  if (!isTargetSource(note, notePath, target)) {
    return false;
  }
  if (target.format === 'epub') {
    return Boolean(target.cfi) && note.cfi === target.cfi;
  }
  const rects = sanitizeRects(target.rects);
  return target.page !== null
    && target.page !== undefined
    && note.page === target.page
    && rects.length > 0
    && sameRects(note.rects, rects);
}

type BookHighlightNoteBlock = {
  start: number;
  end: number;
  note: ParsedBookHighlightNote;
};

function findBookHighlightNoteBlocks(content: string): BookHighlightNoteBlock[] {
  const blocks: BookHighlightNoteBlock[] = [];
  const normalized = content.replace(/\r\n?/g, '\n');
  const fencePattern = /(^|\n)(`{3,}|~{3,})note[^\n]*\n/g;
  let match: RegExpExecArray | null;
  while ((match = fencePattern.exec(normalized)) !== null) {
    const fence = match[2];
    const blockStart = match.index + (match[1] ? 1 : 0);
    const bodyStart = fencePattern.lastIndex;
    const closingPattern = new RegExp(`(^|\\n)${escapeRegExp(fence)}\\s*(?=\\n|$)`, 'g');
    closingPattern.lastIndex = bodyStart;
    const close = closingPattern.exec(normalized);
    if (!close) {
      break;
    }
    const bodyEnd = close.index + (close[1] ? 1 : 0);
    let blockEnd = closingPattern.lastIndex;
    if (normalized[blockEnd] === '\n') {
      blockEnd += 1;
    }
    const parsed = parseBookHighlightNoteBlock(normalized.slice(bodyStart, bodyEnd));
    if (parsed) {
      blocks.push({ start: blockStart, end: blockEnd, note: parsed });
    }
    fencePattern.lastIndex = closingPattern.lastIndex;
  }
  return blocks;
}

export function buildBookHighlightEntry(input: BookHighlightNoteInput): string {
  const createdAt = input.createdAt || new Date();
  const title = (input.sourceTitle || input.sourcePath.split('/').pop() || 'Book').trim();
  const location = (input.locator || '').trim();
  const quote = normalizeNoteBody(input.text);
  const notePath = getBookNotePath(input.sourcePath);
  const source = relativePath(notePath, input.sourcePath);
  const rects = sanitizeRects(input.rects);
  const meta = [
    serializeNoteMeta('type', 'book-highlight'),
    serializeNoteMeta('source', source.startsWith('.') ? source : `./${source}`),
    serializeNoteMeta('title', title),
    serializeNoteMeta('format', input.format),
    serializeNoteMeta('locator', location),
    serializeNoteMeta('cfi', input.format === 'epub' ? input.cfi : null),
    serializeNoteMeta('page', input.format === 'pdf' ? input.page : null),
    rects.length > 0 ? serializeNoteMeta('rects', JSON.stringify(rects)) : null,
    serializeNoteMeta('created', createdAt.toISOString()),
  ].filter((line): line is string => Boolean(line));
  const body = `${meta.join('\n')}\n---\n${quote}`;
  const fence = fenceForContent(body);

  return `${fence}note\n${body}\n${fence}`;
}

export function appendBookHighlightNote(existingContent: string, input: BookHighlightNoteInput): string {
  const existing = existingContent.trimEnd();
  const entry = buildBookHighlightEntry(input);
  if (!existing.trim()) {
    return `${NOTE_HEADING}\n\n${entry}\n`;
  }
  return `${existing}\n\n${entry}\n`;
}

export function hasHighlightText(text: string): boolean {
  return normalizeHighlightText(text).length > 0;
}

export function parseBookHighlightNoteBlock(body: string): ParsedBookHighlightNote | null {
  const normalized = body.replace(/\r\n?/g, '\n').trim();
  const separator = normalized.indexOf('\n---\n');
  if (separator === -1) {
    return null;
  }
  const meta = parseMetaLines(normalized.slice(0, separator).split('\n'));
  if (meta.type !== 'book-highlight') {
    return null;
  }
  const format = meta.format?.toLowerCase();
  if (format !== 'epub' && format !== 'pdf') {
    return null;
  }
  const source = (meta.source || '').trim();
  if (!source) {
    return null;
  }
  const page = meta.page && Number.isFinite(Number(meta.page)) ? Number(meta.page) : null;
  return {
    type: 'book-highlight',
    source,
    title: (meta.title || source.split('/').pop() || 'Book').trim(),
    format,
    locator: (meta.locator || '').trim(),
    cfi: meta.cfi?.trim() || null,
    page,
    rects: parseRects(meta.rects),
    created: (meta.created || '').trim(),
    text: normalized.slice(separator + '\n---\n'.length).trim(),
  };
}

export function parseBookHighlightNotes(content: string): ParsedBookHighlightNote[] {
  return findBookHighlightNoteBlocks(content).map((block) => block.note);
}

export function filterBookHighlightNotesForTarget(
  notes: ParsedBookHighlightNote[],
  target: BookHighlightNoteTarget
): ParsedBookHighlightNote[] {
  const notePath = getBookNotePath(target.sourcePath);
  return notes.filter((note) => (
    isTargetSource(note, notePath, target)
    && isValidHighlightForFormat(note)
  ));
}

export function removeBookHighlightNote(
  existingContent: string,
  target: BookHighlightNoteRemovalTarget
): RemoveBookHighlightNoteResult {
  const notePath = getBookNotePath(target.sourcePath);
  const normalized = existingContent.replace(/\r\n?/g, '\n');
  const blocks = findBookHighlightNoteBlocks(normalized)
    .filter((block) => isRemovalTarget(block.note, notePath, target));
  if (blocks.length === 0) {
    return { content: existingContent, removed: 0 };
  }

  let next = '';
  let cursor = 0;
  for (const block of blocks) {
    next += normalized.slice(cursor, block.start);
    cursor = block.end;
  }
  next += normalized.slice(cursor);
  next = next.replace(/\n{3,}/g, '\n\n').trimEnd();
  if (next) {
    next += '\n';
  }
  return { content: next, removed: blocks.length };
}
