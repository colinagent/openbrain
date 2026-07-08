import { parseCanonicalChatFrontmatter } from '../utils/frontmatterParser';
import { parseFenceLine, isMatchingFenceCloser, type ParsedFenceLine } from '../components/Editor/markdownFences';
import { parseMarkdownImage } from '../utils/markdownMedia';

export type ThinkingLevel = string;
const DEFAULT_THREAD_TITLE = 'Untitled Chat';
const DEFAULT_IMAGE_THREAD_TITLE = 'Image';

export type ChatFrontmatter = {
  threadID: string;
  title?: string;
  parentThreadID?: string;
};

export type ChatFrontmatterPatch = {
  threadID?: string | null;
  title?: string | null;
  parentThreadID?: string | null;
};

export type ChatUserContentPayload = { type: 'text'; text: string };

function normalizeChatUserText(input: string): string {
  return (input || '').replace(/\r\n/g, '\n').trim();
}

function normalizeChatTitleSeedText(input: string): string {
  return filterTopLevelMarkdownLines(input, (trimmed) => (
    parseMarkdownImage(trimmed) != null || isStandaloneMarkdownLinkLine(trimmed)
  ))
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

function filterTopLevelMarkdownLines(
  input: string,
  shouldDrop: (trimmedLine: string) => boolean,
): string {
  const normalized = (input || '').replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  const keptLines: string[] = [];
  let activeFence: ParsedFenceLine | null = null;

  for (const line of lines) {
    if (activeFence) {
      keptLines.push(line);
      if (isMatchingFenceCloser(activeFence, line)) {
        activeFence = null;
      }
      continue;
    }

    const opener = parseFenceLine(line);
    if (opener) {
      activeFence = opener;
      keptLines.push(line);
      continue;
    }

    const trimmed = line.trim();
    if (shouldDrop(trimmed)) {
      continue;
    }
    keptLines.push(line);
  }

  return keptLines.join('\n');
}

function isStandaloneMarkdownLinkLine(text: string): boolean {
  const trimmed = (text || '').trim();
  if (!trimmed || trimmed.startsWith('![')) {
    return false;
  }
  const parsed = parseMarkdownLinkPrefix(trimmed);
  return parsed != null && parsed.length === trimmed.length;
}

function parseMarkdownLinkPrefix(text: string): { length: number } | null {
  if (!text.startsWith('[')) {
    return null;
  }
  const label = parseMarkdownLinkLabel(text, 1);
  if (!label || text[label.next] !== '(') {
    return null;
  }
  const target = parseMarkdownLinkTarget(text, label.next);
  if (!target) {
    return null;
  }
  return { length: target.next };
}

function parseMarkdownLinkLabel(text: string, start: number): { next: number } | null {
  for (let index = start; index < text.length; index += 1) {
    const char = text[index];
    if (char === '\\') {
      if (index + 1 >= text.length) {
        return null;
      }
      index += 1;
      continue;
    }
    if (char === ']') {
      return { next: index + 1 };
    }
    if (char === '\n' || char === '\r') {
      return null;
    }
  }
  return null;
}

function parseMarkdownLinkTarget(text: string, openParenIndex: number): { next: number } | null {
  if (text[openParenIndex] !== '(') {
    return null;
  }
  let depth = 1;
  for (let index = openParenIndex + 1; index < text.length; index += 1) {
    const char = text[index];
    if (char === '\\') {
      if (index + 1 >= text.length) {
        return null;
      }
      index += 1;
      continue;
    }
    if (char === '(') {
      depth += 1;
      continue;
    }
    if (char === ')') {
      depth -= 1;
      if (depth === 0) {
        return { next: index + 1 };
      }
      continue;
    }
    if (char === '\n' || char === '\r') {
      return null;
    }
  }
  return null;
}

export function buildChatCreateTitleSeed(text: string) {
  const normalizedText = normalizeChatTitleSeedText(text);
  if (normalizedText) {
    return normalizedText;
  }
  return '';
}

export function buildChatUserContentPayload(text: string): ChatUserContentPayload {
  const normalizedText = normalizeChatUserText(text);
  return { type: 'text', text: normalizedText };
}

function requireTranscriptUserID(uid?: string | null): string {
  const normalized = (uid || '').trim();
  if (!normalized) {
    throw new Error('uid is required');
  }
  return normalized;
}

function isTranscriptParticipantMarkerLine(line: string): boolean {
  return /^@(user|agent)-[A-Za-z0-9][A-Za-z0-9_-]*$/.test((line || '').trim());
}

function escapeTranscriptParticipantMarkerLines(markdown: string): string {
  return (markdown || '')
    .replace(/\r\n/g, '\n')
    .trim()
    .split('\n')
    .map((line) => {
      if (!isTranscriptParticipantMarkerLine(line)) {
        return line;
      }
      const atIndex = line.indexOf('@');
      return atIndex >= 0 ? `${line.slice(0, atIndex)}\\${line.slice(atIndex)}` : line;
    })
    .join('\n')
    .trim();
}

export function buildChatUserTranscriptChunk(
  payload: ChatUserContentPayload,
  options?: { userID?: string | null },
): string {
  const blocks: string[] = [`@${requireTranscriptUserID(options?.userID)}`];
  const body = escapeTranscriptParticipantMarkerLines(payload.text);
  if (body) {
    blocks.push(body);
  }
  return blocks.join('\n\n');
}

export function isAutoGeneratedChatTitle(title: string | null | undefined): boolean {
  const normalized = (title || '').trim().toLowerCase();
  return normalized === DEFAULT_THREAD_TITLE.toLowerCase() || normalized === DEFAULT_IMAGE_THREAD_TITLE.toLowerCase();
}

export function extractChatThreadID(content: string): string {
  return extractChatFrontmatter(content).threadID;
}

export function extractChatFrontmatter(content: string): ChatFrontmatter {
  const chatFrontmatter = parseCanonicalChatFrontmatter(content);
  return {
    threadID: chatFrontmatter?.threadID || '',
    ...(chatFrontmatter?.title ? { title: chatFrontmatter.title } : {}),
    ...(chatFrontmatter?.parentThreadID ? { parentThreadID: chatFrontmatter.parentThreadID } : {}),
  };
}

function hasPatchKey<T extends object>(patch: T, key: keyof T): boolean {
  return Object.prototype.hasOwnProperty.call(patch, key);
}

function normalizeFrontmatterPatchValue(value: string | null | undefined): string | null {
  const normalized = (value || '').trim();
  return normalized || null;
}

export function updateChatFrontmatter(
  content: string,
  patch: ChatFrontmatterPatch
): string {
  const normalized = content.replace(/\r\n/g, '\n');
  if (!normalized.startsWith('---\n')) {
    return content;
  }
  const end = normalized.indexOf('\n---\n', 4);
  if (end < 0) {
    return content;
  }

  const frontmatterBody = normalized.slice(4, end);
  const rest = normalized.slice(end + 5);
  const lines = frontmatterBody.split('\n');
  const nextLines: string[] = [];
  const seenKeys = new Set<string>();

  const normalizedPatch = {
    thread: normalizeFrontmatterPatchValue(patch.threadID || null),
    title: normalizeFrontmatterPatchValue(patch.title || null),
    parent_thread: normalizeFrontmatterPatchValue(patch.parentThreadID || null),
  } as const;
  const hasExplicitPatch = {
    thread: hasPatchKey(patch, 'threadID'),
    title: hasPatchKey(patch, 'title'),
    parent_thread: hasPatchKey(patch, 'parentThreadID'),
  } as const;

  const patchOrder = ['thread', 'title', 'parent_thread'] as const;

  const formatPatchedLine = (key: typeof patchOrder[number], value: string | null): string | null => {
    if (!value) {
      return null;
    }
    if (key === 'title') {
      return `title: ${JSON.stringify(value)}`;
    }
    return `${key}: ${value}`;
  };

  for (const rawLine of lines) {
    const key = rawLine.split(':', 1)[0]?.trim().toLowerCase();
    if (!key || !patchOrder.includes(key as typeof patchOrder[number])) {
      nextLines.push(rawLine);
      continue;
    }
    const normalizedKey = key as typeof patchOrder[number];
    seenKeys.add(normalizedKey);
    if (!hasExplicitPatch[normalizedKey]) {
      nextLines.push(rawLine);
      continue;
    }
    if (normalizedKey === 'title' && hasPatchKey(patch, 'title') && !normalizedPatch.title) {
      nextLines.push(rawLine);
      continue;
    }
    const nextLine = formatPatchedLine(normalizedKey, normalizedPatch[normalizedKey]);
    if (nextLine) {
      nextLines.push(nextLine);
    }
  }

  for (const key of patchOrder) {
    if (seenKeys.has(key) || !hasExplicitPatch[key]) {
      continue;
    }
    if (key === 'title' && hasPatchKey(patch, 'title') && !normalizedPatch.title) {
      continue;
    }
    const nextLine = formatPatchedLine(key, normalizedPatch[key]);
    if (nextLine) {
      nextLines.push(nextLine);
    }
  }

  return `---\n${nextLines.join('\n')}\n---\n${rest}`;
}
