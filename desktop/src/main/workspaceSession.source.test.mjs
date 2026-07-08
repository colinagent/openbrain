import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const mainPath = path.join(__dirname, 'main.ts');
const source = fs.readFileSync(mainPath, 'utf8');

test('workspace session sync updates the main-process window label from the active workspace tab', () => {
  assert.match(source, /function shouldRegenerateWorkspaceLabel/);
  assert.match(source, /const activeTab = getActiveWorkspaceTabFromSession\(normalized\);/);
  assert.match(source, /record\.info\.label = shouldRegenerateWorkspaceLabel\(activeTab\.label\)/);
});

test('workspace labels are derived from paths without trailing slashes', () => {
  assert.match(source, /workspacePath\.replace\(\s*\/\[\\\\\/\]\+\$\/,\s*''\s*\)/);
});

test('workspace session normalization derives local workspacePath from currentDir', () => {
  assert.match(source, /const rawWorkspacePath = normalizeOptionalString\(entry\.workspacePath\);/);
  assert.match(source, /const currentDir = normalizeOptionalString\(entry\.currentDir\);/);
  assert.match(source, /const workspacePath = rawWorkspacePath \|\| \(kind === 'local' \? currentDir : undefined\);/);
});
