import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const composerPath = path.join(__dirname, 'ChatMarkdownComposer.tsx');
const panelPath = path.join(__dirname, 'ConversationComposerDock.tsx');
const extensionPath = path.join(__dirname, 'composerPlanLineExtension.ts');

test('plan line is rendered as a composer footer widget instead of a top pending resource pill', () => {
  const composerSource = readFileSync(composerPath, 'utf8');
  const panelSource = readFileSync(panelPath, 'utf8');
  const extensionSource = readFileSync(extensionPath, 'utf8');

  assert.match(composerSource, /planBlock: ComposerPlanState \| null;/);
  assert.match(composerSource, /footerWidgetExtension: planBlock/);
  assert.match(composerSource, /onPlanBlockStateChange: \(planState: ComposerPlanState\) => void;/);
  assert.match(panelSource, /displayedSelectedSkill && displayedSelectedSkill\.slug !== 'plan'/);
  assert.match(panelSource, /planBlock=\{!isCommandMode && displayedSelectedSkill\?\.slug === 'plan' \? selectedPlanBlock : null\}/);
  assert.match(panelSource, /onPlanBlockStateChange=\{handlePlanBlockStateChange\}/);
  assert.match(extensionSource, /planState\.anchor/);
  assert.match(extensionSource, /mapComposerPlanState\(value\.planState, tr\.changes\)/);
  assert.match(extensionSource, /onStateChange/);
  assert.match(extensionSource, /label\.textContent = 'Plan'/);
  assert.doesNotMatch(extensionSource, /textContent = '\/plan'/);
});
