import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

function resolveRendererRoot() {
  const fromRepoRoot = path.resolve(process.cwd(), 'desktop/src/renderer');
  const fromAppRoot = path.resolve(process.cwd(), 'src/renderer');
  try {
    if (statSync(fromRepoRoot).isDirectory()) {
      return fromRepoRoot;
    }
  } catch {
    // Fall through to the app-root variant.
  }
  return fromAppRoot;
}

const rendererRoot = resolveRendererRoot();
const nativeSelectPattern = new RegExp(['<', 'select', String.raw`(?=[\s>])`].join(''));

function collectRendererFiles(dir: string, out: string[] = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectRendererFiles(fullPath, out);
      continue;
    }
    if (!/\.(ts|tsx|mjs|mts)$/.test(entry.name)) {
      continue;
    }
    out.push(fullPath);
  }
  return out;
}

test('renderer source no longer uses native select elements', () => {
  const offenders = collectRendererFiles(rendererRoot).filter((filePath) => {
    if (!statSync(filePath).isFile()) {
      return false;
    }
    return nativeSelectPattern.test(readFileSync(filePath, 'utf8'));
  });

  assert.deepEqual(offenders, []);
});
