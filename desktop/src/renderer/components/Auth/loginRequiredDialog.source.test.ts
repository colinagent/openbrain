import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'LoginRequiredDialog.tsx'), 'utf8');

test('LoginRequiredDialog keeps cancel available while opening sign in and exposes copy link fallback', () => {
  assert.match(source, /dialog:auth\.copyLink/);
  assert.match(source, /dialog:auth\.advancedOptions/);
  assert.match(source, /useState<LoginMode>\('organization'\)/);
  assert.match(source, /loginMode === 'organization'/);
  assert.match(source, /loginMode === 'custom-gateway'/);
  assert.match(source, /gateway: loginMode === 'custom-gateway'/);
  assert.match(source, /dialog:auth\.privateServer/);
  assert.match(source, /dialog:auth\.orgCode/);
  assert.match(source, /dialog:auth\.gatewayPlaceholder/);
  assert.match(source, /dialog:auth\.gatewayHelp/);
  assert.match(source, /dialog:auth\.orgHelp/);
  assert.match(source, /orgSlug: loginMode === 'organization'/);
  assert.match(source, /aria-expanded=\{advancedOpen\}/);
  assert.match(source, /advancedOpen \? \(/);
  assert.match(source, /setBusy\(false\)/);
  assert.match(source, /result\?\.mode === 'device_code'/);
  assert.match(source, /onCancel\(\);\s*return;/);
  assert.match(source, /OpenBrainLogo className="h-10 w-10" title="OpenBrain"/);
  assert.doesNotMatch(source, /variant=/);
  assert.doesNotMatch(source, /login-required-sign-in-floor/);
  assert.match(source, /className="dialog-action-btn"[\s\S]*onClick=\{onCancel\}/);
  assert.doesNotMatch(source, /onClick=\{onCancel\}\s*disabled=\{busy\}/);
  assert.match(source, /\$\{OP_SG_CAPSULE\} \$\{OP_SG_CAPSULE_ON_EDITOR\}/);
  assert.doesNotMatch(source, /OP_SG_CAPSULE_PRIMARY/);
  assert.doesNotMatch(source, /bg-button-bg[\s\S]*onClick=\{handleSignIn\}/);
});
