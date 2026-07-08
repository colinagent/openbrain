import {
  DEFAULT_CHAT_MARKDOWN_IMAGE_WIDTH_PERCENT,
  normalizePosixPath,
} from './markdownMedia';
import type { ChatInputImage } from './chatImages';

export type ChatImageAssetWriteResult = {
  success: boolean;
  error?: string;
};

export type WriteChatImageAssetFile = (
  path: string,
  base64: string,
  options?: { overwrite?: boolean }
) => Promise<ChatImageAssetWriteResult>;

export type PersistedChatImageAsset = {
  path: string;
  markdown: string;
};

function normalizeCwd(cwd: string | null | undefined): string {
  const normalized = normalizePosixPath((cwd || '').trim());
  return normalized.startsWith('/') ? normalized.replace(/\/+$/, '') : '';
}

function formatTimestamp(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return [
    date.getFullYear(),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
  ].join('-') + '-'
    + [pad(date.getHours()), pad(date.getMinutes()), pad(date.getSeconds())].join('');
}

function sanitizeFileStem(name: string): string {
  const basename = (name || '').trim().replace(/\\/g, '/').split('/').pop() || 'image';
  const withoutExt = basename.replace(/\.[a-z0-9]+$/i, '');
  return withoutExt
    .replace(/[^a-zA-Z0-9._-]+/g, '-')
    .replace(/[-_.]+$/g, '')
    .replace(/^[-_.]+/g, '')
    .slice(0, 48)
    || 'image';
}

function normalizeImageExtension(extension: string, mimeType: string): string {
  const ext = (extension || '').trim().toLowerCase();
  if (/^\.(png|jpe?g|gif|webp)$/.test(ext)) {
    return ext === '.jpeg' ? '.jpg' : ext;
  }
  switch ((mimeType || '').trim().toLowerCase()) {
    case 'image/jpeg':
      return '.jpg';
    case 'image/gif':
      return '.gif';
    case 'image/webp':
      return '.webp';
    case 'image/png':
    default:
      return '.png';
  }
}

async function sha256Hex(input: string): Promise<string> {
  const encoder = new TextEncoder();
  if (globalThis.crypto?.subtle) {
    const digest = await globalThis.crypto.subtle.digest('SHA-256', encoder.encode(input));
    return Array.from(new Uint8Array(digest))
      .map((item) => item.toString(16).padStart(2, '0'))
      .join('');
  }
  let hash = 2166136261;
  for (const byte of encoder.encode(input)) {
    hash ^= byte;
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
}

function escapeMarkdownLabel(value: string): string {
  return (value || 'image').replace(/([\\\]])/g, '\\$1');
}

function formatMarkdownTarget(path: string): string {
  if (/[\s()]/.test(path)) {
    return `<${path.replace(/>/g, '%3E')}>`;
  }
  return path;
}

export function buildChatImageAssetsDir(cwd: string | null | undefined): string {
  const normalizedCwd = normalizeCwd(cwd);
  return normalizedCwd ? `${normalizedCwd}/.agent/assets/images` : '';
}

export async function persistChatImageAsset(
  image: ChatInputImage,
  cwd: string | null | undefined,
  writeFile: WriteChatImageAssetFile,
  now = new Date(),
): Promise<PersistedChatImageAsset> {
  const assetsDir = buildChatImageAssetsDir(cwd);
  if (!assetsDir) {
    throw new Error('Select an agent before adding images.');
  }

  const extension = normalizeImageExtension(image.extension, image.mimeType);
  const hash = (await sha256Hex(image.base64)).slice(0, 8);
  const stem = sanitizeFileStem(image.name);
  let lastError = '';
  for (let attempt = 0; attempt < 10; attempt += 1) {
    const suffix = attempt === 0 ? '' : `-${attempt + 1}`;
    const fileName = `${formatTimestamp(now)}-${hash}-${stem}${suffix}${extension}`;
    const path = `${assetsDir}/${fileName}`;
    const result = await writeFile(path, image.base64, { overwrite: false });
    if (result.success) {
      return {
        path,
        markdown: `![${escapeMarkdownLabel(fileName)}](${formatMarkdownTarget(path)}){width=${DEFAULT_CHAT_MARKDOWN_IMAGE_WIDTH_PERCENT}%}`,
      };
    }
    lastError = result.error || '';
  }
  throw new Error(lastError || '写入图片文件失败');
}

export async function persistChatImageAssets(
  images: ChatInputImage[],
  cwd: string | null | undefined,
  writeFile: WriteChatImageAssetFile,
): Promise<PersistedChatImageAsset[]> {
  const persisted: PersistedChatImageAsset[] = [];
  for (const image of images) {
    persisted.push(await persistChatImageAsset(image, cwd, writeFile));
  }
  return persisted;
}
