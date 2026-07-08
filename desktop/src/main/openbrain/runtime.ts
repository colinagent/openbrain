import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { setTimeout as delay } from 'node:timers/promises';

const OPENBRAIN_ROOT = '.openbrain';
const RUN_DIR = 'run';
const LATEST_VERSION_FILE = 'latest.version';
const RUNNING_VERSION_FILE = 'running.version';
const PID_FILE = 'openbrain-runtime.pid';
export const MANAGED_SERVER_AGENT_ID = 'agent-openbrain-server';
const execFileAsync = promisify(execFile);

async function exists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

export function getOpenBrainBaseDir(homeDir: string): string {
  return path.join(homeDir, OPENBRAIN_ROOT);
}

export function getOpenBrainRunDir(homeDir: string): string {
  return path.join(getOpenBrainBaseDir(homeDir), RUN_DIR);
}

export function getLatestVersionPath(homeDir: string): string {
  return path.join(getOpenBrainRunDir(homeDir), LATEST_VERSION_FILE);
}

export function getRunningVersionPath(homeDir: string): string {
  return path.join(getOpenBrainRunDir(homeDir), RUNNING_VERSION_FILE);
}

export function getOpenBrainPidPath(homeDir: string): string {
  return path.join(getOpenBrainRunDir(homeDir), PID_FILE);
}

async function readTextFile(filePath: string): Promise<string | null> {
  try {
    const raw = await fs.readFile(filePath, 'utf8');
    const value = raw.trim();
    return value || null;
  } catch {
    return null;
  }
}

async function writeTextFile(filePath: string, value: string): Promise<void> {
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, `${value.trim()}\n`, 'utf8');
}

export async function readLatestInstalledVersion(homeDir: string): Promise<string | null> {
  return readTextFile(getLatestVersionPath(homeDir));
}

export async function readRunningVersion(homeDir: string): Promise<string | null> {
  return readTextFile(getRunningVersionPath(homeDir));
}

export async function writeLatestInstalledVersion(homeDir: string, version: string): Promise<void> {
  await writeTextFile(getLatestVersionPath(homeDir), version);
}

export async function writeRunningVersion(homeDir: string, version: string): Promise<void> {
  await writeTextFile(getRunningVersionPath(homeDir), version);
}

export async function clearRunningVersion(homeDir: string): Promise<void> {
  await fs.rm(getRunningVersionPath(homeDir), { force: true });
}

async function readPid(homeDir: string): Promise<number | null> {
  const raw = await readTextFile(getOpenBrainPidPath(homeDir));
  if (!raw || !/^\d+$/.test(raw)) {
    return null;
  }
  return Number.parseInt(raw, 10);
}

function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

async function findListeningPids(port: number): Promise<number[]> {
  try {
    const { stdout } = await execFileAsync('lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN']);
    return String(stdout || '')
      .split(/\s+/)
      .map((item) => item.trim())
      .filter((item) => /^\d+$/.test(item))
      .map((item) => Number.parseInt(item, 10));
  } catch {
    return [];
  }
}

async function killPid(pid: number): Promise<void> {
  if (!Number.isFinite(pid) || pid <= 0) {
    return;
  }
  if (!isProcessAlive(pid)) {
    return;
  }

  try {
    process.kill(pid, 'SIGTERM');
  } catch {
    return;
  }

  for (let attempt = 0; attempt < 20; attempt += 1) {
    if (!isProcessAlive(pid)) {
      return;
    }
    await delay(200);
  }

  try {
    process.kill(pid, 'SIGKILL');
  } catch {
    // ignore
  }
}

async function killLocalRuntimeSidecars(): Promise<void> {
  const pids = await findListeningPids(19530);
  for (const pid of pids) {
    await killPid(pid);
  }
}

export async function stopManagedOpenBrain(homeDir: string): Promise<void> {
  const pid = await readPid(homeDir);
  if (pid && Number.isFinite(pid)) {
    await killPid(pid);
  }

  await killLocalRuntimeSidecars();
  await fs.rm(getOpenBrainPidPath(homeDir), { force: true });
  await clearRunningVersion(homeDir);
}

export function buildManagedServerAgentMarkdown(commandPath: string, port: number): string {
  return [
    '---',
    `id: ${MANAGED_SERVER_AGENT_ID}`,
    'name: openbrain-server',
    'description: openbrain server (ws + chat) for workspace access',
    'tags: builtin,server,system',
    'opcodes: system/started, notify/message, system/config/get',
    'run:',
    `  command: ["${commandPath}", "--host", "127.0.0.1", "--port", "${port}"]`,
    '  daemon: true',
    '---',
    '',
  ].join('\n');
}

export async function isInstalledVersionReady(homeDir: string, version: string, paths: string[]): Promise<boolean> {
  const installedVersion = await readLatestInstalledVersion(homeDir);
  if (installedVersion !== version) {
    return false;
  }
  for (const filePath of paths) {
    if (!(await exists(filePath))) {
      return false;
    }
  }
  return true;
}
