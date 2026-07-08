import { normalizePosixPath } from './markdownMedia';
import type { ChatSelectionSnapshot } from './chatSelectionPrompt';

function escapeMarkdownLabel(value: string): string {
  return (value || '').replace(/([\\\]])/g, '\\$1');
}

function formatMarkdownTarget(path: string): string {
  if (/[\s()]/.test(path)) {
    return `<${path.replace(/>/g, '%3E')}>`;
  }
  return path;
}

function formatMarkdownLink(label: string, targetPath: string): string {
  const normalizedPath = normalizePosixPath((targetPath || '').trim());
  if (!normalizedPath) {
    return escapeMarkdownLabel(label);
  }
  return `[${escapeMarkdownLabel(label)}](${formatMarkdownTarget(normalizedPath)})`;
}

function extractFileName(path: string): string {
  const normalized = normalizePosixPath((path || '').trim()).replace(/\/+$/, '');
  const parts = normalized.split('/').filter(Boolean);
  return parts[parts.length - 1] || normalized || 'file';
}

export function buildSelectionReferenceLink(
  snapshot: ChatSelectionSnapshot,
  filePath: string | null | undefined,
): string {
  const normalizedPath = normalizePosixPath((filePath || '').trim());
  const lineLabel = snapshot.startLine === snapshot.endLine
    ? `L${snapshot.startLine}`
    : `L${snapshot.startLine}-L${snapshot.endLine}`;
  const label = `${extractFileName(normalizedPath)}#${lineLabel}`;
  return formatMarkdownLink(label, normalizedPath);
}

export function buildFileReferenceLink(path: string, isDir: boolean): string {
  const normalizedPath = normalizePosixPath((path || '').trim());
  const name = extractFileName(normalizedPath);
  const label = isDir ? `directory: ${name}` : `file: ${name}`;
  return formatMarkdownLink(label, normalizedPath);
}
