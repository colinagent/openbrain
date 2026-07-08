import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const mainSource = readFileSync(path.join(__dirname, 'main.ts'), 'utf8');
const bundleScriptSource = readFileSync(
  path.resolve(__dirname, '../../scripts/build-runtime-bundle.sh'),
  'utf8',
);

test('packaged desktop records the bundled runtime version separately from app version', () => {
  assert.match(bundleScriptSource, /runtime-version\.txt/);
  assert.match(bundleScriptSource, /printf '%s\\n' "\$\{RUNTIME_VERSION\}"/);
  assert.match(mainSource, /async function getBundledRuntimeVersion/);
  assert.match(mainSource, /const bundledRuntimeVersion = bundledRuntimeBundlePath \? await getBundledRuntimeVersion\(\) : null;/);
  assert.match(mainSource, /currentVersion: bundledRuntimeVersion \|\| app\.getVersion\(\)/);
});
