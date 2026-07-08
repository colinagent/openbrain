import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { SshHost } from './sshTypes';

type HostRecord = {
  alias: string;
  hostname?: string;
  user?: string;
  port?: string;
  identityFile?: string;
  source?: string;
};

const wildcardRegex = /[*?!\[\]]/;

function expandTilde(input: string) {
  if (input.startsWith('~/')) {
    return path.join(os.homedir(), input.slice(2));
  }
  return input;
}

function isWildcard(alias: string) {
  return wildcardRegex.test(alias);
}

function tokenize(line: string) {
  const trimmed = line.trim();
  if (!trimmed) {
    return null;
  }
  if (trimmed.startsWith('#')) {
    return null;
  }
  const hashIndex = trimmed.indexOf(' #');
  const clean = hashIndex >= 0 ? trimmed.slice(0, hashIndex).trim() : trimmed;
  const [key, ...rest] = clean.split(/\s+/);
  if (!key) {
    return null;
  }
  return { key: key.toLowerCase(), value: rest.join(' ') };
}

function segmentMatches(pattern: string, name: string) {
  const escaped = pattern
    .replace(/[-/\\^$+?.()|{}]/g, '\\$&')
    .replace(/\\\*/g, '.*')
    .replace(/\\\?/g, '.');
  const regex = new RegExp(`^${escaped}$`);
  return regex.test(name);
}

async function expandGlob(pattern: string, baseDir: string): Promise<string[]> {
  const expanded = expandTilde(pattern);
  const resolved = path.isAbsolute(expanded)
    ? expanded
    : path.resolve(baseDir, expanded);

  const root = path.parse(resolved).root;
  const relative = path.relative(root, resolved);
  const segments = relative.split(path.sep).filter(Boolean);

  let paths: string[] = [root];

  for (let i = 0; i < segments.length; i += 1) {
    const segment = segments[i];
    const nextPaths: string[] = [];
    const hasGlob = wildcardRegex.test(segment);

    for (const base of paths) {
      if (!hasGlob) {
        const full = path.join(base, segment);
        try {
          const stat = await fs.stat(full);
          if (stat.isDirectory() || i === segments.length - 1) {
            nextPaths.push(full);
          }
        } catch {
          continue;
        }
      } else {
        try {
          const entries = await fs.readdir(base, { withFileTypes: true });
          for (const entry of entries) {
            if (!segmentMatches(segment, entry.name)) {
              continue;
            }
            const full = path.join(base, entry.name);
            if (entry.isDirectory() || i === segments.length - 1) {
              nextPaths.push(full);
            }
          }
        } catch {
          continue;
        }
      }
    }

    paths = nextPaths;
  }

  return paths;
}

async function parseConfigFile(
  filePath: string,
  hosts: Map<string, HostRecord>,
  visited: Set<string>
) {
  const resolved = path.resolve(filePath);
  if (visited.has(resolved)) {
    return;
  }
  visited.add(resolved);

  let content = '';
  try {
    content = await fs.readFile(resolved, 'utf8');
  } catch {
    return;
  }

  const baseDir = path.dirname(resolved);
  let activeAliases: string[] = [];

  const lines = content.split(/\r?\n/);
  for (const line of lines) {
    const token = tokenize(line);
    if (!token) {
      continue;
    }

    if (token.key === 'include') {
      const patterns = token.value.split(/\s+/).filter(Boolean);
      for (const pattern of patterns) {
        const matches = await expandGlob(pattern, baseDir);
        for (const match of matches) {
          await parseConfigFile(match, hosts, visited);
        }
      }
      continue;
    }

    if (token.key === 'host') {
      const aliases = token.value.split(/\s+/).filter(Boolean);
      activeAliases = aliases.filter((alias) => !isWildcard(alias));
      for (const alias of activeAliases) {
        if (!hosts.has(alias)) {
          hosts.set(alias, { alias, source: resolved });
        }
      }
      continue;
    }

    if (token.key === 'match') {
      activeAliases = [];
      continue;
    }

    if (activeAliases.length === 0) {
      continue;
    }

    for (const alias of activeAliases) {
      const host = hosts.get(alias);
      if (!host) {
        continue;
      }
      switch (token.key) {
        case 'hostname':
          host.hostname = token.value;
          break;
        case 'user':
          host.user = token.value;
          break;
        case 'port':
          host.port = token.value;
          break;
        case 'identityfile':
          host.identityFile = expandTilde(token.value);
          break;
        default:
          break;
      }
    }
  }
}

export async function listSshHosts(): Promise<SshHost[]> {
  const configPath = path.join(os.homedir(), '.ssh', 'config');
  const hosts = new Map<string, HostRecord>();
  const visited = new Set<string>();

  await parseConfigFile(configPath, hosts, visited);

  return Array.from(hosts.values()).sort((a, b) =>
    a.alias.localeCompare(b.alias)
  );
}
