import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appRoot = path.join(__dirname, '..', '..');

function read(relativePath) {
  return readFileSync(path.join(appRoot, relativePath), 'utf8');
}

test('active UI states use highlight instead of a separate active token', () => {
  const rendererTokens = read('src/renderer/theme/tokens.ts');
  const rendererPresets = read('src/renderer/theme/presets.ts');
  const mainSettings = read('src/main/settings/settingsStore.ts');
  const themeTemplate = read('settings/theme.jsonc');
  const tailwindConfig = read('tailwind.config.js');
  const styles = read('src/renderer/styles/index.css');
  const tabLayout = read('src/renderer/components/tabLayout.ts');

  const camelActiveToken = new RegExp(['active', 'Text'].join(''));
  const kebabActiveToken = new RegExp(['active', 'text'].join('-'));
  const cssActiveToken = new RegExp(['--color-active', 'text'].join('-'));

  assert.doesNotMatch(rendererTokens, camelActiveToken);
  assert.doesNotMatch(rendererPresets, camelActiveToken);
  assert.doesNotMatch(mainSettings, camelActiveToken);
  assert.doesNotMatch(themeTemplate, new RegExp(`"${['active', 'Text'].join('')}"`));
  assert.doesNotMatch(tailwindConfig, kebabActiveToken);
  assert.doesNotMatch(styles, cssActiveToken);

  assert.match(rendererPresets, /DEFAULT_LIGHT_CORE/);
  assert.match(rendererPresets, /expandCoreToPalette\(DEFAULT_LIGHT_CORE, 'light'\)/);
  assert.match(mainSettings, /id: 'default-light',[\s\S]*brand: '#be7e4a',/);
  assert.match(themeTemplate, /"id": "default-light",[\s\S]*"obBrand": "#be7e4a"/);
  assert.match(mainSettings, /id: 'openbrain-light',[\s\S]*brand: '#2f8f6b',/);
  assert.match(themeTemplate, /"id": "openbrain-light",[\s\S]*"obBrand": "#2f8f6b"/);
  assert.match(styles, /\.file-tree-item\.selected\s*\{[^}]*color:\s*var\(--color-highlight\);/s);
  assert.match(styles, /\.ui-tabbar \.tab-active-shell:hover,[\s\S]*?color:\s*var\(--color-highlight\);/);
  assert.doesNotMatch(styles, /\.ui-tabbar \.tab-active-shell:hover \.text-highlight/);
  assert.match(tabLayout, /isActive \? 'tab-active-shell text-highlight' : 'text-secondary-text'/);
  assert.match(read('src/renderer/theme/index.ts'), /obBrand/);
  assert.match(styles, /\.op-markdown-editor \.cm-line\.cm-md-frontmatter-line\.cm-activeLine/);
  assert.match(rendererPresets, /previewFrontmatterBg: pv\?\.frontmatterBg \?\? \(scheme === 'dark' \? darkSurfaceBg : palette\.logoLight\)/);
  assert.match(rendererPresets, /editorActiveLine: ed\?\.activeLine \?\? \(scheme === 'dark' \? darkActiveLineBg : palette\.logoLight\)/);
});
