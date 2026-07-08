import * as fs from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { createWriteStream } from 'node:fs';
import { promisify } from 'node:util';
import * as path from 'node:path';
import { tmpdir } from 'node:os';
import { createHash } from 'node:crypto';
import { Readable, Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import * as tar from 'tar';
import { parse as parseYaml } from 'yaml';
import type { OpenBrainOs, ReleasePlatformAssets } from './releaseManifest';
import {
  buildManagedServerAgentMarkdown,
  isInstalledVersionReady,
  writeLatestInstalledVersion,
} from './runtime';

const execFileAsync = promisify(execFile);
const openBrainPathStartMarker = '# >>> OpenBrain managed PATH >>>';
const openBrainPathEndMarker = '# <<< OpenBrain managed PATH <<<';

async function exists(p: string): Promise<boolean> {
  try {
    await fs.access(p);
    return true;
  } catch {
    return false;
  }
}

async function ensureDir(p: string) {
  await fs.mkdir(p, { recursive: true });
}

function getExeSuffix(os: OpenBrainOs) {
  return os === 'windows' ? '.exe' : '';
}

async function downloadWithSha256(
  url: string,
  outFile: string,
  expectedSha256: string,
): Promise<void> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(
      `Download failed: ${res.status} ${res.statusText} (${url})`,
    );
  }
  if (!res.body) {
    throw new Error(`Download response has no body (${url})`);
  }

  await ensureDir(path.dirname(outFile));

  const hash = createHash('sha256');
  const hasher = new Transform({
    transform(chunk, _enc, cb) {
      hash.update(chunk);
      cb(null, chunk);
    },
  });

  try {
    await pipeline(
      Readable.fromWeb(res.body as any),
      hasher,
      createWriteStream(outFile),
    );
  } catch (e) {
    await fs.rm(outFile, { force: true });
    throw e;
  }

  const actual = hash.digest('hex');
  if (actual.toLowerCase() !== expectedSha256.toLowerCase()) {
    await fs.rm(outFile, { force: true });
    throw new Error(
      `SHA256 mismatch for ${url}: expected ${expectedSha256}, got ${actual}`,
    );
  }
}

async function extractTarGz(archivePath: string, extractDir: string) {
  await ensureDir(extractDir);
  await tar.x({ file: archivePath, cwd: extractDir });
}

async function copyDirForce(src: string, dst: string) {
  if (!(await exists(src))) {
    return;
  }
  await ensureDir(dst);
  await (fs as any).cp(src, dst, { recursive: true, force: true });
}

function parseManifestTags(markdown: string): string[] {
  const text = markdown.replace(/\r\n/g, '\n');
  const lines = text.split('\n');
  if (lines[0]?.trim() !== '---') {
    return [];
  }
  const end = lines.findIndex(
    (line, index) => index > 0 && ['---', '...'].includes(line.trim()),
  );
  if (end < 0) {
    return [];
  }
  const raw = parseYaml(lines.slice(1, end).join('\n')) as
    | { tags?: unknown }
    | null;
  return normalizeManifestTags(raw?.tags);
}

function normalizeManifestTags(value: unknown): string[] {
  const parts: string[] = [];
  if (typeof value === 'string') {
    parts.push(...value.split(','));
  } else if (Array.isArray(value)) {
    for (const item of value) {
      if (typeof item === 'string') {
        parts.push(...item.split(','));
      }
    }
  }
  return parts.map((item) => item.trim()).filter(Boolean);
}

function userPathEntryForDir(dir: string, homeDir: string, os: OpenBrainOs) {
  const cleanDir = path.resolve(dir);
  const cleanHome = path.resolve(homeDir);
  const rel = path.relative(cleanHome, cleanDir);
  if (rel === path.join('.openbrain', 'bin')) {
    return os === 'windows'
      ? `%USERPROFILE%\\${rel.replace(/\//g, '\\')}`
      : `$HOME/${rel.replace(/\\/g, '/')}`;
  }
  return cleanDir;
}

function unixProfilePaths(homeDir: string): string[] {
  const names = ['.profile'];
  const shellName = path.basename(process.env.SHELL || '');
  if (shellName === 'zsh') {
    names.push('.zprofile', '.zshrc');
  } else if (shellName === 'bash') {
    names.push('.bash_profile', '.bashrc');
  }
  return Array.from(new Set(names)).map((name) => path.join(homeDir, name));
}

function unixPathBlock(entry: string) {
  return `${openBrainPathStartMarker}
case ":$PATH:" in
  *":${entry}:"*) ;;
  *) export PATH="$PATH:${entry}" ;;
esac
${openBrainPathEndMarker}
`;
}

async function upsertManagedPathBlock(profilePath: string, block: string) {
  let current = '';
  try {
    current = await fs.readFile(profilePath, 'utf8');
  } catch {
    current = '';
  }
  const start = current.indexOf(openBrainPathStartMarker);
  const end = current.indexOf(openBrainPathEndMarker);
  let next: string;
  if (start >= 0 && end >= start) {
    next = current.slice(0, start) + block + current.slice(end + openBrainPathEndMarker.length);
  } else {
    const separator = current.trim() && !current.endsWith('\n') ? '\n' : '';
    next = current + separator + block;
  }
  await ensureDir(path.dirname(profilePath));
  await fs.writeFile(profilePath, next, 'utf8');
}

async function ensureUnixUserPathContains(dir: string, homeDir: string) {
  const entry = userPathEntryForDir(dir, homeDir, 'linux');
  const block = unixPathBlock(entry);
  await Promise.all(
    unixProfilePaths(homeDir).map((profilePath) =>
      upsertManagedPathBlock(profilePath, block),
    ),
  );
}

function psSingleQuote(value: string) {
  return `'${value.replace(/'/g, "''")}'`;
}

async function ensureWindowsUserPathContains(dir: string, homeDir: string) {
  const entry = userPathEntryForDir(dir, homeDir, 'windows');
  const script = `
$entry = ${psSingleQuote(entry)}
$expandedEntry = [Environment]::ExpandEnvironmentVariables($entry)
$current = [Environment]::GetEnvironmentVariable('Path', 'User')
if ([string]::IsNullOrWhiteSpace($current)) {
  [Environment]::SetEnvironmentVariable('Path', $entry, 'User')
  exit
}
$exists = $false
foreach ($part in $current -split [IO.Path]::PathSeparator) {
  $trimmed = $part.Trim()
  if ($trimmed.Length -eq 0) { continue }
  try {
    $expanded = [Environment]::ExpandEnvironmentVariables($trimmed)
    if ([IO.Path]::GetFullPath($expanded).TrimEnd('\\') -ieq [IO.Path]::GetFullPath($expandedEntry).TrimEnd('\\')) {
      $exists = $true
      break
    }
  } catch {}
}
if (-not $exists) {
  [Environment]::SetEnvironmentVariable('Path', "$current$([IO.Path]::PathSeparator)$entry", 'User')
}
`;
  await execFileAsync('powershell.exe', [
    '-NoProfile',
    '-NonInteractive',
    '-ExecutionPolicy',
    'Bypass',
    '-Command',
    script,
  ]);
}

async function ensureUserPathContains(dir: string, homeDir: string, os: OpenBrainOs) {
  if (os === 'windows') {
    await ensureWindowsUserPathContains(dir, homeDir);
    return;
  }
  await ensureUnixUserPathContains(dir, homeDir);
}

async function copyProjectedBinFile(
  sourcePath: string,
  targetPath: string,
  mode: number,
  os: OpenBrainOs,
) {
  await ensureDir(path.dirname(targetPath));
  await fs.copyFile(sourcePath, targetPath);
  if (os !== 'windows') {
    await fs.chmod(targetPath, mode || 0o755);
  }
}

async function toolManifestHasSystemTag(toolDir: string) {
  try {
    const text = await fs.readFile(path.join(toolDir, 'TOOL.md'), 'utf8');
    return parseManifestTags(text).some((tag) => tag.toLowerCase() === 'system');
  } catch {
    return false;
  }
}

async function projectSystemToolBins(opRoot: string, homeDir: string, os: OpenBrainOs) {
  const toolsRoot = path.join(opRoot, 'tools');
  const runtimeBinDir = path.join(opRoot, 'bin');
  let copied = false;
  if (!(await exists(toolsRoot))) {
    return;
  }
  const entries = await fs.readdir(toolsRoot, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory()) {
      continue;
    }
    const toolDir = path.join(toolsRoot, entry.name);
    if (!(await toolManifestHasSystemTag(toolDir))) {
      continue;
    }
    const sourceBinDir = path.join(toolDir, 'bin');
    if (!(await exists(sourceBinDir))) {
      continue;
    }
    const binEntries = await fs.readdir(sourceBinDir, { withFileTypes: true });
    for (const binEntry of binEntries) {
      if (!binEntry.isFile()) {
        continue;
      }
      const sourcePath = path.join(sourceBinDir, binEntry.name);
      const info = await fs.stat(sourcePath);
      await copyProjectedBinFile(
        sourcePath,
        path.join(runtimeBinDir, binEntry.name),
        info.mode & 0o777,
        os,
      );
      copied = true;
    }
  }
  if (copied) {
    await ensureUserPathContains(runtimeBinDir, homeDir, os);
  }
}

export async function ensureLocalInstalledFromRelease(options: {
  homeDir: string;
  os: OpenBrainOs;
  port: number;
  version: string;
  assets: ReleasePlatformAssets;
}) {
  const opRoot = path.join(options.homeDir, '.openbrain');
  const runtimeBinDir = path.join(opRoot, 'bin');
  const agentsRoot = path.join(opRoot, 'agents');
  const toolsRoot = path.join(opRoot, 'tools');
  const skillsRoot = path.join(opRoot, 'skills');
  const serverRoot = path.join(agentsRoot, 'opagent-server');
  const serverAgentDir = path.join(serverRoot, '.agent');
  const serverBinDir = path.join(serverAgentDir, 'bin');
  const coderManifestTarget = path.join(
    agentsRoot,
    'coder',
    '.agent',
    'AGENT.md',
  );
  const simpleMemoryManifestTarget = path.join(
    agentsRoot,
    'simple-memory',
    '.agent',
    'AGENT.md',
  );
  const gbrainManifestTarget = path.join(
    agentsRoot,
    'gbrain',
    '.agent',
    'AGENT.md',
  );
  const gbrainCloudToolManifestTarget = path.join(
    toolsRoot,
    'gbrain-cloud',
    'TOOL.md',
  );
  const openBrainCloudSyncSkillManifestTarget = path.join(
    skillsRoot,
    'openbrain-cloud-sync',
    'SKILL.md',
  );

  const exe = getExeSuffix(options.os);
  const openbrainTarget = path.join(runtimeBinDir, `opagent-runtime${exe}`);
  const gbrainTarget = path.join(runtimeBinDir, `gbrain${exe}`);
  const serverTarget = path.join(serverBinDir, `openbrain-server${exe}`);
  const configTarget = path.join(opRoot, 'configs', 'config.json');
  const agentFile = path.join(serverAgentDir, 'AGENT.md');
  const coderTarget = path.join(
    agentsRoot,
    'coder',
    '.agent',
    'bin',
    `coder${exe}`,
  );
  const openBrainCloudSyncHelperTarget = path.join(
    skillsRoot,
    'openbrain-cloud-sync',
    'bin',
    `openbrain-cloud-sync-helper${exe}`,
  );

  await ensureDir(runtimeBinDir);
  await ensureDir(serverBinDir);
  await ensureDir(serverAgentDir);

  const alreadyInstalled = await isInstalledVersionReady(
    options.homeDir,
    options.version,
    [
      openbrainTarget,
      gbrainTarget,
      serverTarget,
      coderTarget,
      coderManifestTarget,
      simpleMemoryManifestTarget,
      gbrainManifestTarget,
      gbrainCloudToolManifestTarget,
      openBrainCloudSyncSkillManifestTarget,
      openBrainCloudSyncHelperTarget,
      configTarget,
    ],
  );

  if (!alreadyInstalled) {
    const workDir = await fs.mkdtemp(path.join(tmpdir(), `openbrain-bundle-`));
    const archivePath = path.join(workDir, 'bundle.tar.gz');
    const extractedDir = path.join(workDir, 'x');
    try {
      await downloadWithSha256(
        options.assets.bundle.url,
        archivePath,
        options.assets.bundle.sha256,
      );
      await extractTarGz(archivePath, extractedDir);

      await copyDirForce(path.join(extractedDir, 'bin'), runtimeBinDir);
      await copyDirForce(
        path.join(extractedDir, 'agents', 'coder'),
        path.join(agentsRoot, 'coder'),
      );
      await copyDirForce(
        path.join(extractedDir, 'agents', 'simple-memory'),
        path.join(agentsRoot, 'simple-memory'),
      );
      await copyDirForce(
        path.join(extractedDir, 'agents', 'gbrain'),
        path.join(agentsRoot, 'gbrain'),
      );
      await copyDirForce(
        path.join(extractedDir, 'agents', 'opagent-server', '.agent', 'bin'),
        serverBinDir,
      );
      await copyDirForce(path.join(extractedDir, 'tools'), toolsRoot);
      await copyDirForce(
        path.join(extractedDir, 'skills', 'openbrain-cloud-sync'),
        path.join(skillsRoot, 'openbrain-cloud-sync'),
      );

      if (!(await exists(configTarget))) {
        const cfgSrc = path.join(extractedDir, 'configs', 'config.json');
        if (await exists(cfgSrc)) {
          await ensureDir(path.dirname(configTarget));
          await fs.copyFile(cfgSrc, configTarget);
        }
      }
    } finally {
      await fs.rm(workDir, { recursive: true, force: true });
    }

    if (options.os !== 'windows') {
      if (await exists(openbrainTarget)) {
        await fs.chmod(openbrainTarget, 0o755);
      }
      if (await exists(serverTarget)) {
        await fs.chmod(serverTarget, 0o755);
      }
      if (await exists(gbrainTarget)) {
        await fs.chmod(gbrainTarget, 0o755);
      }
      if (await exists(openBrainCloudSyncHelperTarget)) {
        await fs.chmod(openBrainCloudSyncHelperTarget, 0o755);
      }
    }

    await writeLatestInstalledVersion(options.homeDir, options.version);
  }
  await projectSystemToolBins(opRoot, options.homeDir, options.os);

  await fs.writeFile(
    agentFile,
    buildManagedServerAgentMarkdown(
      `./bin/openbrain-server${exe}`,
      options.port,
    ),
    'utf8',
  );

  return {
    openbrainBinary: openbrainTarget,
    configPath: configTarget,
    version: options.version,
  };
}
