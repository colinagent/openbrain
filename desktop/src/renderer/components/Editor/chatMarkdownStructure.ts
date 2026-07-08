import { parseFenceLine } from './markdownFences';

export type OutlineEntry = {
  id: string;
  type: 'heading' | 'user-message';
  level: number;
  text: string;
  pos: number;
  spyPos: number;
  userMessageId: string | null;
};

export type OutlineTreeEntry = OutlineEntry & {
  parentId: string | null;
  depth: number;
  hasChildren: boolean;
  ancestorIds: string[];
  topLevelId: string | null;
};

export type ParsedLine = {
  number: number;
  from: number;
  text: string;
};

const MAX_HEADING_LABEL_LENGTH = 50;
const MAX_USER_LABEL_LENGTH = 20;
const GLOBAL_OUTLINE_GROUP_ID = '__global__';

export function trimOutlineLabel(text: string, maxLength = MAX_HEADING_LABEL_LENGTH): string {
  const normalized = text.trim().replace(/\s+/g, ' ');
  if (normalized.length <= maxLength) {
    return normalized;
  }
  if (maxLength <= 3) {
    return normalized.slice(0, maxLength);
  }
  return `${normalized.slice(0, maxLength - 3)}...`;
}

export function stripHeadingImages(text: string): string {
  return text
    .replace(/!\[[^\]]*\]\([^)]+\)(?:\{[^}]+\})?\s*/gu, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function normalizeUserLabelText(text: string): string {
  const normalized = text.replace(/\r\n/g, '\n');
  const cleanedLines = normalized.split('\n').map((line) => {
    let next = line;
    if (/^\s*(```+|~~~+)/.test(next)) {
      return ' ';
    }
    next = next.replace(/^\s{0,3}>\s?/g, '');
    next = next.replace(/^\s{0,3}(?:[-+*]|\d+\.)\s+/g, '');
    next = next.replace(/^\s{0,3}#{1,6}\s+/g, '');
    return next;
  });

  return cleanedLines.join(' ')
    .replace(/!\[([^\]]*)\]\([^)]+\)(?:\{[^}]+\})?/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/<[^>\n]+>/g, ' ')
    .replace(/[>*_~]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

export function formatUserMessageLabel(text: string): string {
  const plain = normalizeUserLabelText(text);
  if (!plain) {
    return 'User message';
  }
  return trimOutlineLabel(plain, MAX_USER_LABEL_LENGTH);
}

export function getLinesFromContent(content: string): ParsedLine[] {
  const rawLines = content.split('\n');
  const lines: ParsedLine[] = [];
  let from = 0;
  for (let i = 0; i < rawLines.length; i++) {
    const text = rawLines[i];
    lines.push({
      number: i + 1,
      from,
      text,
    });
    from += text.length + 1;
  }
  return lines;
}

function isParticipantMarkerLine(text: string): boolean {
  return /^@(user|agent)-[A-Za-z0-9][A-Za-z0-9_-]*$/.test((text || '').trim());
}

function isUserMarkerLine(text: string): boolean {
  return /^@user-[A-Za-z0-9][A-Za-z0-9_-]*$/.test((text || '').trim());
}

function findUserMarkerLabel(lines: ParsedLine[], startIndex: number): { text: string; pos: number } {
  for (let index = startIndex; index < lines.length; index += 1) {
    const line = lines[index];
    const trimmed = line.text.trim();
    if (!trimmed) {
      continue;
    }
    if (isParticipantMarkerLine(trimmed)) {
      break;
    }
    if (parseFenceLine(line.text)) {
      return { text: 'User message', pos: line.from };
    }
    const firstTextIndex = line.text.search(/\S/);
    return {
      text: formatUserMessageLabel(line.text),
      pos: line.from + (firstTextIndex >= 0 ? firstTextIndex : 0),
    };
  }
  const fallback = lines[Math.max(0, startIndex - 1)];
  return { text: 'User message', pos: fallback?.from ?? 0 };
}

export function parseOutlineEntries(lines: ParsedLine[]): OutlineEntry[] {
  const entries: OutlineEntry[] = [];
  let inFence = false;
  let fenceChar = '';
  let fenceWidth = 0;
  let inFrontmatter = lines.length > 0 && lines[0].text.trim() === '---';
  let frontmatterClosed = !inFrontmatter;
  let activeUserMessageId: string | null = null;

  for (let index = 0; index < lines.length; index += 1) {
    const parsedLine = lines[index];
    const line = parsedLine.text;
    const trimmed = line.trim();

    if (inFrontmatter) {
      if (parsedLine.number > 1 && (trimmed === '---' || trimmed === '...')) {
        inFrontmatter = false;
        frontmatterClosed = true;
      }
      continue;
    }

    if (!frontmatterClosed && trimmed === '---') {
      inFrontmatter = true;
      continue;
    }

    if (!inFence && isUserMarkerLine(trimmed)) {
      const id = `user-${parsedLine.from}`;
      const label = findUserMarkerLabel(lines, index + 1);
      entries.push({
        id,
        type: 'user-message',
        level: 0,
        text: label.text,
        pos: label.pos,
        spyPos: parsedLine.from,
        userMessageId: null,
      });
      activeUserMessageId = id;
      continue;
    }

    if (!inFence && isParticipantMarkerLine(trimmed)) {
      continue;
    }

    const fence = parseFenceLine(line);
    if (fence) {
      if (!inFence) {
        inFence = true;
        fenceChar = fence.char;
        fenceWidth = fence.width;
      } else if (
        fence.char === fenceChar
        && fence.width >= fenceWidth
        && fence.info === ''
      ) {
        inFence = false;
        fenceChar = '';
        fenceWidth = 0;
      }
      continue;
    }

    if (inFence) {
      continue;
    }

    const headingMatch = line.match(/^(#{1,6})\s+(.*)$/);
    if (headingMatch) {
      const headingText = trimOutlineLabel(stripHeadingImages(headingMatch[2]));
      if (headingText) {
        const headingPos = parsedLine.from + headingMatch[1].length + 1;
        entries.push({
          id: `heading-${headingPos}`,
          type: 'heading',
          level: headingMatch[1].length,
          text: headingText,
          pos: headingPos,
          spyPos: parsedLine.from,
          userMessageId: activeUserMessageId,
        });
      }
    }
  }
  return entries;
}

export function buildOutlineTreeEntries(entries: OutlineEntry[]): OutlineTreeEntry[] {
  const treeEntries: OutlineTreeEntry[] = [];
  const entryMap = new Map<string, OutlineTreeEntry>();
  const headingStack: OutlineTreeEntry[] = [];
  let currentGroupId = GLOBAL_OUTLINE_GROUP_ID;

  for (const entry of entries) {
    if (entry.type === 'user-message') {
      const treeEntry: OutlineTreeEntry = {
        ...entry,
        parentId: null,
        depth: 0,
        hasChildren: false,
        ancestorIds: [],
        topLevelId: entry.id,
      };
      treeEntries.push(treeEntry);
      entryMap.set(treeEntry.id, treeEntry);
      headingStack.length = 0;
      currentGroupId = entry.id;
      continue;
    }

    const nextGroupId = entry.userMessageId ?? GLOBAL_OUTLINE_GROUP_ID;
    if (nextGroupId !== currentGroupId) {
      headingStack.length = 0;
      currentGroupId = nextGroupId;
    }

    while (headingStack.length > 0 && headingStack[headingStack.length - 1].level >= entry.level) {
      headingStack.pop();
    }

    const rootParent = entry.userMessageId ? entryMap.get(entry.userMessageId) ?? null : null;
    const parent = headingStack[headingStack.length - 1] ?? rootParent;
    const treeEntry: OutlineTreeEntry = {
      ...entry,
      parentId: parent?.id ?? null,
      depth: parent ? parent.depth + 1 : 0,
      hasChildren: false,
      ancestorIds: parent ? [...parent.ancestorIds, parent.id] : [],
      topLevelId: parent ? (parent.topLevelId ?? parent.id) : entry.id,
    };

    if (parent) {
      parent.hasChildren = true;
    }

    treeEntries.push(treeEntry);
    entryMap.set(treeEntry.id, treeEntry);
    headingStack.push(treeEntry);
  }

  return treeEntries;
}
