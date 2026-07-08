import fs from 'fs';
import * as fsp from 'fs/promises';
import path from 'path';
import { createHash } from 'node:crypto';
export type ConfigSyncPushFile = {
  name: string;
  content: string;
};

export type ConfigSyncPushPayload = {
  files: ConfigSyncPushFile[];
};

type DispatchConfigSyncPush = (payload: ConfigSyncPushPayload) => void;

const DEBOUNCE_MS = 300;
const SYNC_CONFIG_FILES = new Set(['auth.json', 'models.json', 'nodes.json', 'profile.json']);

let watcher: fs.FSWatcher | null = null;
let currentUserDir: string | null = null;
let debounceTimers = new Map<string, NodeJS.Timeout>();
let dispatchPush: DispatchConfigSyncPush | null = null;
let lastPushedBatchHash: string | null = null;

function hashContent(content: string): string {
  return createHash('sha256').update(content).digest('hex');
}

function hashBatch(files: Array<{ name: string; content: string }>): string {
  const stable = files
    .map((f) => `${f.name}:${hashContent(f.content)}`)
    .sort()
    .join('|');
  return hashContent(stable);
}

async function readAllUserJsonFiles(dir: string): Promise<ConfigSyncPushFile[]> {
  const names = await listUserFiles(dir);
  const files: ConfigSyncPushFile[] = [];
  for (const name of names) {
    const filePath = path.join(dir, name);
    try {
      const content = await fsp.readFile(filePath, 'utf8');
      files.push({ name, content });
    } catch (err) {
      if ((err as NodeJS.ErrnoException)?.code === 'ENOENT') {
        continue;
      }
      console.warn('[configSync] read failed:', filePath, err);
    }
  }
  return files;
}

async function pushCurrentConfigBatch(force = false): Promise<void> {
  if (!currentUserDir || !dispatchPush) return;
  const files = await readAllUserJsonFiles(currentUserDir);
  const batchHash = hashBatch(files);
  if (!force && lastPushedBatchHash === batchHash) {
    return;
  }
  lastPushedBatchHash = batchHash;
  dispatchPush({ files });
}

function scheduleSync(filename: string) {
  const existing = debounceTimers.get(filename);
  if (existing) clearTimeout(existing);
  debounceTimers.set(filename, setTimeout(() => {
    debounceTimers.delete(filename);
    pushCurrentConfigBatch(false).catch((err) => {
      console.warn('[configSync] sync failed:', filename, err);
    });
  }, DEBOUNCE_MS));
}

async function listUserFiles(dir: string): Promise<string[]> {
  try {
    const entries = await fsp.readdir(dir);
    return entries.filter((name) => SYNC_CONFIG_FILES.has(name));
  } catch {
    return [];
  }
}

export async function syncAllToTarget(opts?: { force?: boolean }): Promise<void> {
  await pushCurrentConfigBatch(Boolean(opts?.force));
}

export async function startConfigSync(homeDir: string, onPush: DispatchConfigSyncPush): Promise<void> {
  stopConfigSync();
  dispatchPush = onPush;
  const userDir = path.join(homeDir, '.openbrain', 'configs', 'user');
  await fsp.mkdir(userDir, { recursive: true });
  currentUserDir = userDir;

  watcher = fs.watch(userDir, (eventType, filename) => {
    if (!filename) return;
    const name = filename.toString();
    if (!SYNC_CONFIG_FILES.has(name)) return;
    if (eventType === 'rename' || eventType === 'change') {
      scheduleSync(name);
    }
  });
  watcher.on('error', (err) => {
    console.warn('[configSync] watcher error:', err);
  });
}

export function stopConfigSync(): void {
  if (watcher) {
    try { watcher.close(); } catch {}
    watcher = null;
  }
  for (const timer of debounceTimers.values()) clearTimeout(timer);
  debounceTimers.clear();
  dispatchPush = null;
  lastPushedBatchHash = null;
  currentUserDir = null;
}
