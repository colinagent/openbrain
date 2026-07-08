import type { EditorState } from '@codemirror/state';
import {
  buildOutlineTreeEntries,
  getLinesFromContent,
  parseOutlineEntries,
  type OutlineTreeEntry,
} from '../components/Editor/chatMarkdownStructure';
import { getFrontmatterInfo } from '../components/Editor/codemirror/utils/frontmatter';
import { normalizePosixPath } from './markdownMedia';

type BaseSelectionSnapshot = {
  text: string;
  from: number;
  to: number;
  startLine: number;
  endLine: number;
};

export type TextChatSelectionSnapshot = BaseSelectionSnapshot & {
  kind: 'text';
};

export type MarkdownChatSelectionSnapshot = BaseSelectionSnapshot & {
  kind: 'markdown';
  breadcrumb: string[];
};

export type ChatSelectionSnapshot =
  | TextChatSelectionSnapshot
  | MarkdownChatSelectionSnapshot;

const MARKDOWN_ROOT_BREADCRUMB = 'Document Root';
const MARKDOWN_FRONTMATTER_BREADCRUMB = 'Frontmatter';

function getNonEmptySelectionSnapshot(state: EditorState): BaseSelectionSnapshot | null {
  const selection = state.selection.main;
  if (!selection || selection.empty) {
    return null;
  }
  const from = selection.from;
  const to = selection.to;
  const text = state.doc.sliceString(from, to);
  if (!text) {
    return null;
  }
  const startLine = state.doc.lineAt(from).number;
  const endLine = state.doc.lineAt(Math.max(from, to - 1)).number;
  return {
    text,
    from,
    to,
    startLine,
    endLine,
  };
}

function findActiveOutlineTreeEntry(entries: OutlineTreeEntry[], pos: number): OutlineTreeEntry | null {
  if (entries.length === 0) {
    return null;
  }
  let low = 0;
  let high = entries.length - 1;
  let result = -1;
  while (low <= high) {
    const mid = Math.floor((low + high) / 2);
    if (entries[mid].spyPos <= pos) {
      result = mid;
      low = mid + 1;
    } else {
      high = mid - 1;
    }
  }
  return result >= 0 ? entries[result] : null;
}

function buildMarkdownBreadcrumb(state: EditorState, from: number): string[] {
  const frontmatter = getFrontmatterInfo(state);
  if (frontmatter && from >= frontmatter.from && from <= frontmatter.to) {
    return [MARKDOWN_FRONTMATTER_BREADCRUMB];
  }

  const content = state.doc.toString();
  const treeEntries = buildOutlineTreeEntries(parseOutlineEntries(getLinesFromContent(content)));
  const activeEntry = findActiveOutlineTreeEntry(treeEntries, from);
  if (!activeEntry) {
    return [MARKDOWN_ROOT_BREADCRUMB];
  }

  const entryMap = new Map<string, OutlineTreeEntry>();
  for (const entry of treeEntries) {
    entryMap.set(entry.id, entry);
  }

  const labels = [...activeEntry.ancestorIds, activeEntry.id]
    .map((id) => entryMap.get(id)?.text.trim() || '')
    .filter(Boolean);

  return labels.length > 0 ? labels : [MARKDOWN_ROOT_BREADCRUMB];
}

function getMaxConsecutiveBackticks(text: string): number {
  let max = 0;
  const matches = text.match(/`+/g) || [];
  for (const match of matches) {
    if (match.length > max) {
      max = match.length;
    }
  }
  return max;
}

function wrapInlineCode(value: string): string {
  const normalized = value.trim();
  if (!normalized) {
    return '` `';
  }
  const ticks = '`'.repeat(Math.max(1, getMaxConsecutiveBackticks(normalized) + 1));
  return `${ticks}${normalized}${ticks}`;
}

function resolveFenceLanguage(
  filePath: string,
  snapshot: ChatSelectionSnapshot,
): string {
  if (snapshot.kind === 'markdown') {
    return 'markdown';
  }
  const lastSegment = filePath.split('/').pop() || '';
  const dotIndex = lastSegment.lastIndexOf('.');
  if (dotIndex < 0 || dotIndex === lastSegment.length - 1) {
    return '';
  }
  const extension = lastSegment.slice(dotIndex + 1).trim().toLowerCase();
  return /^[a-z0-9_+-]+$/i.test(extension) ? extension : '';
}

function formatLineRange(startLine: number, endLine: number): string {
  return startLine === endLine ? `${startLine}` : `${startLine}-${endLine}`;
}

export function buildTextChatSelectionSnapshot(state: EditorState): TextChatSelectionSnapshot | null {
  const selection = getNonEmptySelectionSnapshot(state);
  if (!selection) {
    return null;
  }
  return {
    kind: 'text',
    ...selection,
  };
}

export function buildMarkdownChatSelectionSnapshot(state: EditorState): MarkdownChatSelectionSnapshot | null {
  const selection = getNonEmptySelectionSnapshot(state);
  if (!selection) {
    return null;
  }
  return {
    kind: 'markdown',
    ...selection,
    breadcrumb: buildMarkdownBreadcrumb(state, selection.from),
  };
}

export function appendChatSelectionToDraft(existingDraft: string, selectionBlock: string): string {
  if (!existingDraft.trim()) {
    return selectionBlock;
  }
  if (existingDraft.endsWith('\n\n')) {
    return `${existingDraft}${selectionBlock}`;
  }
  if (existingDraft.endsWith('\n')) {
    return `${existingDraft}\n${selectionBlock}`;
  }
  return `${existingDraft}\n\n${selectionBlock}`;
}

export function buildChatSelectionPrompt(
  snapshot: ChatSelectionSnapshot,
  filePath: string | null | undefined,
): string {
  const normalizedPath = normalizePosixPath((filePath || '').trim());
  const fenceTicks = '`'.repeat(Math.max(3, getMaxConsecutiveBackticks(snapshot.text) + 1));
  const fenceLanguage = resolveFenceLanguage(normalizedPath, snapshot);
  const openingFence = fenceLanguage ? `${fenceTicks}${fenceLanguage}` : fenceTicks;

  if (snapshot.kind === 'markdown') {
    const breadcrumb = snapshot.breadcrumb.length > 0
      ? snapshot.breadcrumb.join(' > ')
      : MARKDOWN_ROOT_BREADCRUMB;
    const lines = [
      normalizedPath
        ? `Selection from ${wrapInlineCode(normalizedPath)}`
        : 'Selection from current markdown document',
      `Section: ${wrapInlineCode(breadcrumb)}`,
      `Source range: ${wrapInlineCode(`${snapshot.from}-${snapshot.to}`)}`,
    ];
    return `${lines.join('\n')}\n\n${openingFence}\n${snapshot.text}\n${fenceTicks}`;
  }

  const lineRange = formatLineRange(snapshot.startLine, snapshot.endLine);
  const sourceLabel = normalizedPath
    ? `${normalizedPath}:${lineRange}`
    : `current document:${lineRange}`;
  return `Selection from ${wrapInlineCode(sourceLabel)}\n\n${openingFence}\n${snapshot.text}\n${fenceTicks}`;
}
