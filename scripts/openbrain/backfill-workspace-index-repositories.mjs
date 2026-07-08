#!/usr/bin/env node
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const indexPath = process.argv[2]
  ? path.resolve(process.argv[2])
  : path.join(os.homedir(), '.openbrain', 'index', 'workspaces.json');

function parseGitHubRemoteURL(remote) {
  const value = String(remote || '').trim();
  if (!value) {
    return null;
  }
  try {
    const parsed = new URL(value);
    if ((parsed.protocol === 'https:' || parsed.protocol === 'http:' || parsed.protocol === 'ssh:') && parsed.hostname.toLowerCase() === 'github.com') {
      const parts = parsed.pathname.replace(/^\/+|\/+$/g, '').split('/');
      if (parts.length >= 2) {
        return { owner: parts[0], name: parts[1].replace(/\.git$/i, ''), remoteURL: value };
      }
    }
  } catch {
    // Fall through to the scp-like GitHub remote form.
  }
  if (value.startsWith('git@github.com:')) {
    const parts = value.replace(/^git@github\.com:/, '').replace(/\.git$/i, '').split('/');
    if (parts.length === 2) {
      return { owner: parts[0], name: parts[1], remoteURL: value };
    }
  }
  return null;
}

function inspectGitHubRepository(workspacePath) {
  const resolved = path.resolve(String(workspacePath || '').trim());
  if (!resolved || !existsSync(resolved) || !statSync(resolved).isDirectory()) {
    return null;
  }
  try {
    execFileSync('git', ['-C', resolved, 'rev-parse', '--is-inside-work-tree'], { stdio: ['ignore', 'pipe', 'ignore'] });
    const remote = execFileSync('git', ['-C', resolved, 'remote', 'get-url', 'origin'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
    return parseGitHubRemoteURL(remote);
  } catch {
    return null;
  }
}

if (!existsSync(indexPath)) {
  console.error(`Workspace index not found: ${indexPath}`);
  process.exit(1);
}

const index = JSON.parse(readFileSync(indexPath, 'utf8'));
const workspaces = Array.isArray(index.workspaces) ? index.workspaces : [];
const now = new Date().toISOString();
let changed = 0;

for (const entry of workspaces) {
  const repo = inspectGitHubRepository(entry.path);
  if (!repo) {
    continue;
  }
  const current = entry.repository && typeof entry.repository === 'object' ? entry.repository : {};
  const nextRepository = {
    ...current,
    enabled: true,
    provider: 'github',
    owner: repo.owner,
    name: repo.name,
    remoteURL: repo.remoteURL,
    webURL: `https://github.com/${repo.owner}/${repo.name}`,
  };
  if (JSON.stringify(current) === JSON.stringify(nextRepository)) {
    continue;
  }
  entry.repository = nextRepository;
  entry.updatedAt = now;
  changed += 1;
}

if (changed === 0) {
  console.log(`No workspace repository metadata changed in ${indexPath}`);
  process.exit(0);
}

writeFileSync(indexPath, `${JSON.stringify({ version: index.version || 1, workspaces, hiddenWorkspaces: index.hiddenWorkspaces || [] }, null, 2)}\n`);
console.log(`Updated repository metadata for ${changed} workspace binding(s) in ${indexPath}`);
