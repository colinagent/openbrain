import * as fs from 'fs/promises';
import * as path from 'path';
import { nativeImage, net } from 'electron';
import { writeJsonFileAtomic } from './shared/jsonFile';

type OpNodeLike = {
  id: string;
  hostID?: string;
  uid?: string;
  kind?: string;
  uri?: string;
  cwd?: string;
  tags?: string[];
  opCodes?: unknown[];
  run?: Record<string, unknown>;
  meta?: Record<string, unknown>;
};

type NodesJsonData = Record<string, OpNodeLike>;

const AVATAR_SIZE = 64;
const AVATAR_PALETTE: Array<[number, number, number]> = [
  [99, 102, 241],   // indigo
  [16, 185, 129],   // emerald
  [245, 158, 11],   // amber
  [244, 63, 94],    // rose
  [14, 165, 233],   // sky
  [168, 85, 247],   // purple
  [234, 88, 12],    // orange
  [20, 184, 166],   // teal
];

function ensurePathSegment(value: string): string {
  const trimmed = (value || '').trim();
  return trimmed.replace(/^\/+|\/+$/g, '');
}

async function fileExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

function normalizeHostID(value: string): string {
  const normalized = (value || '').trim();
  return normalized || 'default';
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function isOpNodeLike(value: unknown): value is OpNodeLike {
  if (!isObjectRecord(value)) return false;
  return typeof value.id === 'string' && value.id.trim().length > 0;
}

function fallbackNodeId(node: OpNodeLike): string {
  const raw = (node.id || '').trim();
  if (raw) {
    return raw;
  }
  return 'node-unknown';
}

function colorFromText(input: string): [number, number, number] {
  const text = (input || '').trim();
  if (!text) return AVATAR_PALETTE[0];
  let hash = 0;
  for (let i = 0; i < text.length; i++) {
    hash = (hash * 31 + text.charCodeAt(i)) >>> 0;
  }
  return AVATAR_PALETTE[hash % AVATAR_PALETTE.length];
}

function initialsFromName(name: string): string {
  const text = (name || '').trim();
  if (!text) return 'A';
  const parts = text
    .split(/[\s\-_]+/g)
    .map((part) => part.trim())
    .filter(Boolean);
  const firstRune = (value: string): string => [...value][0] || '';
  if (parts.length >= 2) {
    return `${firstRune(parts[0])}${firstRune(parts[parts.length - 1])}`.toUpperCase();
  }
  const chars = [...text];
  if (/[\u3400-\u9fff\uf900-\ufaff]/.test(text)) {
    return chars.slice(-2).join('');
  }
  return chars.slice(0, 2).join('').toUpperCase() || 'A';
}

function escapeXml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

function buildFallbackAvatarSvg(name: string): string {
  const [r, g, b] = colorFromText(name);
  const initials = escapeXml(initialsFromName(name));
  const charCount = [...initials].length;
  const fontSize = charCount <= 1 ? 42 : 34;
  return [
    `<svg xmlns="http://www.w3.org/2000/svg" width="${AVATAR_SIZE}" height="${AVATAR_SIZE}" viewBox="0 0 ${AVATAR_SIZE} ${AVATAR_SIZE}">`,
    `<circle cx="${AVATAR_SIZE / 2}" cy="${AVATAR_SIZE / 2}" r="${AVATAR_SIZE / 2}" fill="rgb(${r}, ${g}, ${b})" />`,
    `<text x="50%" y="52%" text-anchor="middle" dominant-baseline="middle" fill="#ffffff" font-size="${fontSize}" font-weight="700" font-family="system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif">${initials}</text>`,
    '</svg>',
  ].join('');
}

function needsPngFallbackUpgrade(remoteUrl: string, localAvatarPath: string): boolean {
  if (remoteUrl) return false;
  const local = (localAvatarPath || '').trim().toLowerCase();
  return local.endsWith('.png');
}

function resolveDisplayName(node: OpNodeLike): string {
  const meta = (node.meta || {}) as Record<string, unknown>;
  const name = typeof meta.name === 'string' ? meta.name.trim() : '';
  if (name) return name;
  return (node.id || '').trim() || 'Agent';
}

function resolveRemoteAvatar(node: OpNodeLike): string {
  const meta = (node.meta || {}) as Record<string, unknown>;
  const remote = typeof meta.avatar === 'string' ? meta.avatar.trim() : '';
  return remote;
}

function upsertNodeWithLocalAvatar(node: OpNodeLike, localAvatar: string): OpNodeLike {
  const meta = (node.meta || {}) as Record<string, unknown>;
  return {
    ...node,
    id: fallbackNodeId(node),
    meta: {
      ...meta,
      localAvatar,
    },
  };
}

export function getAvatarsDir(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'resources', 'avatars');
}

export function getNodesJsonPath(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'configs', 'user', 'nodes.json');
}

async function ensureAvatarsDir(homeDir: string): Promise<string> {
  const dir = getAvatarsDir(homeDir);
  await fs.mkdir(dir, { recursive: true });
  return dir;
}

async function ensureNodesJsonDir(homeDir: string): Promise<void> {
  const filePath = getNodesJsonPath(homeDir);
  await fs.mkdir(path.dirname(filePath), { recursive: true });
}

export async function loadNodesJson(homeDir: string): Promise<NodesJsonData> {
  const filePath = getNodesJsonPath(homeDir);
  try {
    const content = await fs.readFile(filePath, 'utf8');
    const parsed = JSON.parse(content) as unknown;
    if (!isObjectRecord(parsed)) {
      return {};
    }

    // Format: { [nodeID]: OpNodeLike }
    const asFlat = parsed as Record<string, unknown>;
    const allFlatNodes = Object.values(asFlat).every((value) => isOpNodeLike(value));
    if (allFlatNodes) {
      return asFlat as NodesJsonData;
    }
    return {};
  } catch {
    return {};
  }
}

export async function saveNodesJson(homeDir: string, data: NodesJsonData): Promise<void> {
  await ensureNodesJsonDir(homeDir);
  const filePath = getNodesJsonPath(homeDir);
  await writeJsonFileAtomic(filePath, data);
}

export async function upsertNodes(homeDir: string, hostID: string, nodes: OpNodeLike[]): Promise<void> {
  if (!Array.isArray(nodes) || nodes.length === 0) {
    return;
  }
  const normalizedHostID = normalizeHostID(hostID);
  const current = await loadNodesJson(homeDir);
  let changed = false;

  for (const node of nodes) {
    if (!node || typeof node !== 'object') continue;
    const nodeId = fallbackNodeId(node);
    if (nodeId === 'node-unknown') {
      continue;
    }
    const prev = current[nodeId];
    const prevMeta = (prev?.meta || {}) as Record<string, unknown>;
    const nextMeta = (node.meta || {}) as Record<string, unknown>;

    const nextNode: OpNodeLike = {
      ...prev,
      ...node,
      id: nodeId,
      hostID: normalizedHostID,
      meta: {
        ...prevMeta,
        ...nextMeta,
      },
    };
    const prevJson = prev ? JSON.stringify(prev) : '';
    const nextJson = JSON.stringify(nextNode);
    if (prevJson !== nextJson) {
      current[nodeId] = nextNode;
      changed = true;
    }
  }

  if (!changed) {
    return;
  }

  await saveNodesJson(homeDir, current);
}

async function fetchRemoteAvatar(remoteUrl: string): Promise<Buffer> {
  const res = await net.fetch(remoteUrl);
  if (!res.ok) {
    throw new Error(`Avatar fetch failed: ${res.status}`);
  }
  const ab = await res.arrayBuffer();
  return Buffer.from(ab);
}

export async function downloadAvatar(remoteUrl: string, nodeId: string, homeDir: string): Promise<string> {
  const avatarsDir = await ensureAvatarsDir(homeDir);
  const finalNodeId = ensurePathSegment(nodeId) || 'node-unknown';
  const avatarPath = path.join(avatarsDir, `${finalNodeId}.png`);
  const imageBytes = await fetchRemoteAvatar(remoteUrl);
  const image = nativeImage.createFromBuffer(imageBytes);
  if (image.isEmpty()) {
    throw new Error('Invalid avatar image data');
  }
  const resized = image.resize({ width: AVATAR_SIZE, height: AVATAR_SIZE });
  await fs.writeFile(avatarPath, resized.toPNG());
  return avatarPath;
}

export async function generateFallbackAvatar(name: string, nodeId: string, homeDir: string): Promise<string> {
  const avatarsDir = await ensureAvatarsDir(homeDir);
  const finalNodeId = ensurePathSegment(nodeId) || 'node-unknown';
  const avatarPath = path.join(avatarsDir, `${finalNodeId}.svg`);
  const svg = buildFallbackAvatarSvg(name);
  await fs.writeFile(avatarPath, svg, 'utf8');
  return avatarPath;
}

export async function ensureAvatar(
  remoteUrl: string | null | undefined,
  name: string,
  nodeId: string,
  homeDir: string
): Promise<string> {
  const url = (remoteUrl || '').trim();
  if (url) {
    try {
      return await downloadAvatar(url, nodeId, homeDir);
    } catch {
      // Fall through to deterministic local fallback.
    }
  }
  return generateFallbackAvatar(name, nodeId, homeDir);
}

export async function cacheNodeAvatar(homeDir: string, hostID: string, node: OpNodeLike): Promise<OpNodeLike> {
  const nodeId = fallbackNodeId(node);
  if (nodeId === 'node-unknown') {
    throw new Error('Node id is required');
  }
  const normalizedHostID = normalizeHostID(hostID);
  const displayName = resolveDisplayName(node);
  const remoteAvatar = resolveRemoteAvatar(node);
  const current = await loadNodesJson(homeDir);
  const previous = current[nodeId];
  const previousMeta = (previous?.meta || {}) as Record<string, unknown>;
  const previousRemote = typeof previousMeta.avatar === 'string' ? previousMeta.avatar.trim() : '';
  const previousLocal = typeof previousMeta.localAvatar === 'string' ? previousMeta.localAvatar.trim() : '';
  const shouldUpgradeFallback = needsPngFallbackUpgrade(remoteAvatar, previousLocal);

  if (
    previousRemote === remoteAvatar &&
    previousLocal &&
    !shouldUpgradeFallback &&
    await fileExists(previousLocal)
  ) {
    const merged = {
      ...previous,
      ...node,
      id: nodeId,
      hostID: normalizedHostID,
      meta: {
        ...previousMeta,
        ...(node.meta || {}),
        localAvatar: previousLocal,
      },
    };
    const previousJson = previous ? JSON.stringify(previous) : '';
    const mergedJson = JSON.stringify(merged);
    if (previousJson !== mergedJson) {
      current[nodeId] = merged;
      await saveNodesJson(homeDir, current);
    }
    return merged;
  }

  const localAvatar = await ensureAvatar(remoteAvatar, displayName, nodeId, homeDir);
  const updated = upsertNodeWithLocalAvatar(
    {
      ...previous,
      ...node,
      id: nodeId,
      hostID: normalizedHostID,
      meta: {
        ...previousMeta,
        ...(node.meta || {}),
      },
    },
    localAvatar
  );
  current[nodeId] = updated;
  await saveNodesJson(homeDir, current);
  return updated;
}
