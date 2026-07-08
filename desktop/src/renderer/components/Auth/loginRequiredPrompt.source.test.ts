import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'LoginRequiredPrompt.tsx'), 'utf8');

test('LoginRequiredPrompt opens the shared login required dialog', () => {
  assert.match(source, /showLoginRequiredDialog/);
  assert.match(source, /type LoginRequiredReason/);
  assert.match(source, /onClick=\{\(\) => showLoginRequiredDialog\(reason\)\}/);
  assert.match(source, /LogInIcon/);
  assert.match(source, /OP_SG_CAPSULE/);
  assert.match(source, /substrate === 'sidebar'/);
  assert.match(source, /login-required-prompt-sign-in/);
  assert.doesNotMatch(source, /text-highlight/);
  assert.doesNotMatch(source, /border-active-border/);
  assert.doesNotMatch(source, /startLogin/);
  assert.doesNotMatch(source, /from ['"]\.\/LoginRequiredDialog['"]/);
});
