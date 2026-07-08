#!/usr/bin/env node

import fs from 'node:fs/promises';
import path from 'node:path';

const args = process.argv.slice(2);

let rootDir = process.cwd();
let write = false;
let userToken = 'user';
let agentToken = 'agent';

for (let i = 0; i < args.length; i += 1) {
  const arg = args[i];
  if (arg === '--write') {
    write = true;
    continue;
  }
  if (arg === '--user-token') {
    userToken = sanitizeToken(args[i + 1] || userToken, 'user');
    i += 1;
    continue;
  }
  if (arg === '--agent-token') {
    agentToken = sanitizeToken(args[i + 1] || agentToken, 'agent');
    i += 1;
    continue;
  }
  if (!arg.startsWith('--')) {
    rootDir = path.resolve(arg);
  }
}

async function main() {
  const files = [];
  await collectFiles(rootDir, files);
  const summary = {
    scanned: files.length,
    changed: 0,
    replacedUserHeadings: 0,
    replacedAgentHeadings: 0,
    written: 0,
  };

  for (const filePath of files) {
    const content = await fs.readFile(filePath, 'utf8');
    const result = migrateContent(content, userToken, agentToken);
    if (!result.changed) {
      continue;
    }
    summary.changed += 1;
    summary.replacedUserHeadings += result.replacedUserHeadings;
    summary.replacedAgentHeadings += result.replacedAgentHeadings;

    if (write) {
      await fs.writeFile(filePath, result.nextContent, 'utf8');
      summary.written += 1;
    }
  }

  const modeLabel = write ? 'write mode' : 'dry-run mode';
  console.log(`[thread-marker-migration] ${modeLabel}`);
  console.log(`[thread-marker-migration] root: ${rootDir}`);
  console.log(`[thread-marker-migration] scanned files: ${summary.scanned}`);
  console.log(`[thread-marker-migration] changed files: ${summary.changed}`);
  console.log(`[thread-marker-migration] replaced ## User: ${summary.replacedUserHeadings}`);
  console.log(`[thread-marker-migration] replaced ## Agent: ${summary.replacedAgentHeadings}`);
  if (write) {
    console.log(`[thread-marker-migration] written files: ${summary.written}`);
  } else {
    console.log('[thread-marker-migration] no files were modified (use --write to apply)');
  }
}

function sanitizeToken(rawToken, fallback) {
  const token = String(rawToken || '').trim().replace(/^@+/, '').replace(/\s+/g, '');
  return token || fallback;
}

function isThreadMarkdownFile(filePath) {
  const normalized = filePath.split(path.sep).join('/');
  return normalized.includes('/.agent/threads/') && normalized.endsWith('.md');
}

async function collectFiles(dir, out) {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.name === '.git' || entry.name === 'node_modules' || entry.name === 'dist') {
      continue;
    }
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      await collectFiles(fullPath, out);
      continue;
    }
    if (entry.isFile() && isThreadMarkdownFile(fullPath)) {
      out.push(fullPath);
    }
  }
}

function migrateContent(content, nextUserToken, nextAgentToken) {
  const lines = content.split('\n');
  let replacedUserHeadings = 0;
  let replacedAgentHeadings = 0;

  const nextLines = lines.map((line) => {
    if (/^\s*#{1,6}\s*User\s*$/i.test(line)) {
      replacedUserHeadings += 1;
      return `@${nextUserToken}`;
    }
    if (/^\s*#{1,6}\s*Agent\s*$/i.test(line)) {
      replacedAgentHeadings += 1;
      return `@${nextAgentToken}`;
    }
    return line;
  });

  const changed = replacedUserHeadings > 0 || replacedAgentHeadings > 0;
  return {
    changed,
    replacedUserHeadings,
    replacedAgentHeadings,
    nextContent: changed ? nextLines.join('\n') : content,
  };
}

main().catch((err) => {
  console.error('[thread-marker-migration] failed:', err);
  process.exitCode = 1;
});
