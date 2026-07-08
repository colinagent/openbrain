import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.join(__dirname, 'agentSwitch.ts');

test('agent target formatter names the empty cwd as the default workspace', () => {
  const source = readFileSync(sourcePath, 'utf8');

  assert.match(source, /export const DEFAULT_AGENT_TARGET_WORKSPACE_LABEL = 'workspace';/);
  assert.match(source, /return `\$\{DEFAULT_AGENT_TARGET_WORKSPACE_LABEL\}:\$\{label\}`;/);
  assert.match(source, /return `\$\{DEFAULT_AGENT_TARGET_WORKSPACE_LABEL\} · \$\{label\}`;/);
});
