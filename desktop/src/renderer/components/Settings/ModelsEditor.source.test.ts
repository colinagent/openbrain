import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

function resolveModelsEditorPath() {
  const fromRepoRoot = path.resolve(
    process.cwd(),
    'desktop/src/renderer/components/Settings/ModelsEditor.tsx'
  );
  const fromAppRoot = path.resolve(process.cwd(), 'src/renderer/components/Settings/ModelsEditor.tsx');
  try {
    return readFileSync(fromRepoRoot, 'utf8') ? fromRepoRoot : fromAppRoot;
  } catch {
    return fromAppRoot;
  }
}

const modelsEditorSource = readFileSync(resolveModelsEditorPath(), 'utf8');

test('ModelsEditor no longer exposes Supports thinking in the provider form', () => {
  assert.doesNotMatch(modelsEditorSource, /Supports thinking/);
});

test('ModelsEditor no longer exposes a free-text provider model id field', () => {
  assert.doesNotMatch(modelsEditorSource, /placeholder="Model ID \(required\)"/);
});

test('ModelsEditor exposes an explicit login CTA for OpenBrain models', () => {
  assert.match(modelsEditorSource, /t\('settings:models\.signInRequired'\)/);
  assert.match(modelsEditorSource, /t\('settings:models\.signInHint'\)/);
  assert.match(modelsEditorSource, /await startLogin\(\)/);
  assert.match(modelsEditorSource, /t\('settings:models\.logIn'\)/);
});

test('ModelsEditor keeps provider creation user-facing instead of exposing internal config copy', () => {
  assert.match(modelsEditorSource, /placeholder="Provider name"/);
  assert.match(modelsEditorSource, /Target provider/);
  assert.match(modelsEditorSource, /Log in to load OpenBrain models/);
  assert.match(modelsEditorSource, /deriveProviderKeyFromLabel\(providerLabel\)/);
  assert.match(modelsEditorSource, />\s*Add model\s*</);
  assert.doesNotMatch(modelsEditorSource, /Provider key \(required\)/);
  assert.doesNotMatch(modelsEditorSource, /provider:modelID/);
  assert.doesNotMatch(modelsEditorSource, /User-defined provider/);
  assert.doesNotMatch(modelsEditorSource, /Use this for local or direct provider endpoints only\./);
  assert.doesNotMatch(modelsEditorSource, /Create provider \+ add model/);
});


test('ModelsEditor shows a compact Refresh action beside the Models title', () => {
  assert.match(modelsEditorSource, /<div className="flex items-center gap-2">[\s\S]*<div className="text-lg font-semibold text-prime-text">\{t\('settings:models\.title'\)\}<\/div>[\s\S]*\{t\('settings:models\.refresh'\)\}/);
  assert.doesNotMatch(modelsEditorSource, /flex items-start justify-between gap-3/);
  assert.doesNotMatch(modelsEditorSource, /Refresh from OpenBrain/);
  assert.doesNotMatch(modelsEditorSource, /Uses your OpenBrain session to fetch the built-in provider catalog\./);
});

test('ModelsEditor renders provider-grouped management UI without Auto special-casing', () => {
  assert.match(modelsEditorSource, /Available providers/);
  assert.match(modelsEditorSource, /Edit provider/);
  assert.match(modelsEditorSource, /Remove provider/);
  assert.match(modelsEditorSource, /sortedModels\.filter\(\(model\) => model\.provider === providerKey\)/);
  assert.match(modelsEditorSource, /sortedModels\.filter\(\(model\) => model\.provider === OPENBRAIN_PROVIDER_KEY\)/);
  assert.doesNotMatch(modelsEditorSource, /Special selection/);
});

test('ModelsEditor does not expose Auto as an inline completion model mode', () => {
  assert.match(modelsEditorSource, /ariaLabel="Completion mode"/);
  assert.doesNotMatch(modelsEditorSource, /value: 'auto'/);
  assert.doesNotMatch(modelsEditorSource, /label: 'Auto'/);
  assert.match(modelsEditorSource, /value: 'default', label: 'Default'/);
});

test('ModelsEditor prevents disabling or removing the Default Chat Model', () => {
  assert.match(modelsEditorSource, /const isDefaultChat = \(config\.strategies\?\.auto\?\.defaultChatModelID \|\| ''\)\.trim\(\) === model\.key;/);
  assert.match(modelsEditorSource, /const disableToggle = model\.enabled && isDefaultChat;/);
  assert.match(modelsEditorSource, /disabled=\{disableToggle\}/);
  assert.match(modelsEditorSource, /default chat/);
  assert.match(modelsEditorSource, /setDefaultChatModel\(model\.key\)/);
  assert.match(modelsEditorSource, /Set chat default/);
  assert.doesNotMatch(modelsEditorSource, /onClick=\{\(\) => setDefault\(model\.key\)\}/);
  assert.match(modelsEditorSource, /const providerHasDefaultChat = provider\.models\.some/);
  assert.match(modelsEditorSource, /disabled=\{providerHasDefaultChat\}/);
});

test('ModelsEditor gives the Provider API menu extra width for readable option copy', () => {
  assert.match(modelsEditorSource, /const PROVIDER_API_MENU_CLASS_NAME = 'w-\[360px\] max-w-\[calc\(100vw-32px\)\]';/);
  assert.match(
    modelsEditorSource,
    /menuClassName=\{PROVIDER_API_MENU_CLASS_NAME\}[\s\S]*ariaLabel="Provider API"/
  );
  assert.match(modelsEditorSource, /title: `\$\{option\.label\} — \$\{option\.helperText\}`/);
});

test('ModelsEditor defaults newly added OpenAI models to Responses API', () => {
  assert.match(modelsEditorSource, /const DEFAULT_OPENAI_MODEL_API: ModelEntry\['api'\] = 'openai-responses';/);
  assert.match(modelsEditorSource, /useState<ModelEntry\['api'\]>\(DEFAULT_OPENAI_MODEL_API\)/);
  assert.match(modelsEditorSource, /const nextModel = openbrainModels\.find\(\(model\) => model\.id === nextModelID\);[\s\S]*setNewApi\(nextModel\.api\);/);
  assert.doesNotMatch(modelsEditorSource, /setNewApi\([^)]*'openai-completions'[^)]*\)/);
});
