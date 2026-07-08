import { isValidThreadID } from './threadLink';
export type ParsedFrontmatterValues = {
  thread?: string;
  index?: string;
  title?: string;
  parent_thread?: string;
};

export type CanonicalChatFrontmatter = {
  threadID: string;
  title: string;
  parentThreadID?: string;
};

export function parseFrontmatterStringValue(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return '';
  }
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    try {
      const decoded = JSON.parse(trimmed);
      return typeof decoded === 'string' ? decoded.trim() : trimmed;
    } catch {
      return trimmed.slice(1, -1).trim();
    }
  }
  if (trimmed.startsWith("'") && trimmed.endsWith("'")) {
    return trimmed.slice(1, -1).trim();
  }
  return trimmed;
}

function parseFrontmatterRecord(content: string): Record<string, string> {
  const text = typeof content === 'string' ? content.replace(/\r\n/g, '\n') : '';
  if (!text) {
    return {};
  }
  const lines = text.split('\n');
  if (lines.length < 2 || lines[0].trim() !== '---') {
    return {};
  }
  const result: Record<string, string> = {};
  const maxFrontmatterLines = Math.min(lines.length, 40);
  for (let i = 1; i < maxFrontmatterLines; i += 1) {
    const line = lines[i].trim();
    if (!line) {
      continue;
    }
    if (line === '---' || line === '...') {
      break;
    }
    const match = line.match(/^([a-zA-Z0-9_-]+):\s*(.*)$/);
    if (!match) {
      continue;
    }
    const key = match[1].toLowerCase();
    const value = parseFrontmatterStringValue(match[2]);
    if (!value) {
      continue;
    }
    result[key] = value;
  }
  return result;
}

export function parseThreadFrontmatterValue(raw: string | null | undefined): string {
  const value = parseFrontmatterStringValue(raw || '');
  return isValidThreadID(value) ? value : '';
}

export function getFrontmatterValue(content: string, key: string): string | undefined {
  const wantedKey = (key || '').trim().toLowerCase();
  if (!wantedKey) {
    return undefined;
  }
  const record = parseFrontmatterRecord(content);
  return record[wantedKey];
}

export function parseFrontmatterValues(content: string): ParsedFrontmatterValues {
  const record = parseFrontmatterRecord(content);
  const thread = record.thread;
  const index = record.index;
  const title = record.title;
  const parentThread = record.parent_thread;
  return {
    ...(thread ? { thread } : {}),
    ...(index ? { index } : {}),
    ...(title ? { title } : {}),
    ...(parentThread ? { parent_thread: parentThread } : {}),
  };
}

export function normalizeCanonicalChatFrontmatter(
  value: ParsedFrontmatterValues | null | undefined
): CanonicalChatFrontmatter | null {
  const threadID = parseThreadFrontmatterValue(value?.thread);
  const title = (value?.title || '').trim();
  if (!threadID || !title) {
    return null;
  }
  const parentThreadID = parseThreadFrontmatterValue(value?.parent_thread);
  return {
    threadID,
    title,
    ...(parentThreadID ? { parentThreadID } : {}),
  };
}

export function parseCanonicalChatFrontmatter(content: string): CanonicalChatFrontmatter | null {
  return normalizeCanonicalChatFrontmatter(parseFrontmatterValues(content));
}

export function isCanonicalChatMarkdownContent(content: string): boolean {
  return parseCanonicalChatFrontmatter(content) !== null;
}

export function isConversationMarkdownContent(content: string): boolean {
  const values = parseFrontmatterValues(content);
  return Boolean(parseThreadFrontmatterValue(values.thread) || (values.index || '').trim());
}
