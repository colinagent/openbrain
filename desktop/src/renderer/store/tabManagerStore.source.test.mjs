import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const tabManagerSourcePath = new URL('./tabManagerStore.ts', import.meta.url);
const tabManagerSource = await readFile(tabManagerSourcePath, 'utf8');
const appSourcePath = new URL('../App.tsx', import.meta.url);
const appSource = await readFile(appSourcePath, 'utf8');

test('workspace tab labels are regenerated from real local workspace paths', () => {
  assert.match(tabManagerSource, /function shouldRegenerateLabel/);
  assert.match(tabManagerSource, /normalized\.replace\(\/\\\/\+\$\/,\s*''\)/);
  assert.match(tabManagerSource, /const workspacePath = tab\.workspacePath \|\| \(tab\.kind === 'local' \? currentDir \|\| undefined : undefined\);/);
  assert.match(tabManagerSource, /const label = shouldRegenerateLabel\(tab\.label\)\s*\?\s*resolveTabLabel\(tab\.kind, workspacePath, tab\.remoteHost\)/s);
});

test('restored local tabs bind the tab workspace to currentDir when only currentDir exists', () => {
  assert.match(appSource, /const nextDir = tab\.currentDir \|\| tab\.workspacePath;/);
  assert.match(appSource, /if \(nextDir\) \{\s*useTabManagerStore\.getState\(\)\.updateTabWorkspace\(tab\.id,\s*\{\s*kind: 'local',\s*workspacePath: nextDir,/s);
});

