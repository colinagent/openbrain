import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const conversationComposerDockPath = path.join(__dirname, 'ConversationComposerDock.tsx');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('composer dock binds the plan shortcut globally and keeps composer logic local', () => {
  const source = read(conversationComposerDockPath);
  const composerSection = source.slice(
    source.indexOf('const handleComposerKeyDownCapture = useCallback'),
    source.indexOf('const handleComposerPasteCapture = useCallback'),
  );

  assert.match(source, /const handleGlobalPlanSkillHotkey = useCallback\(\(event: KeyboardEvent\) => \{/);
  assert.match(source, /window\.addEventListener\('keydown', handleGlobalPlanSkillHotkey, true\);/);
  assert.match(source, /window\.removeEventListener\('keydown', handleGlobalPlanSkillHotkey, true\);/);
  assert.match(source, /className="flex min-w-0 items-center gap-1 overflow-x-auto overflow-y-hidden pr-2"/);
  assert.match(source, /if \(hasBlockingModal\) \{\s*return undefined;\s*\}/m);
  assert.match(source, /const activateSelectedPlanSkill = useCallback\(/);
  assert.match(source, /const removeActivePlanBlock = useCallback\(/);
  assert.match(source, /if \(option\.slug === 'plan'\) \{\s*activateSelectedPlanSkill/m);
  assert.match(source, /removeActivePlanBlock\(\{ clearSkill: false \}\);/);
  assert.match(source, /displayedSelectedSkill\?\.slug !== 'plan' \|\| !selectedTargetKey \|\| selectedPlanBlock/);
  assert.doesNotMatch(composerSection, /resolvePlanSkillShortcutAction/);
  assert.doesNotMatch(composerSection, /Plan skill is still loading/);
  assert.doesNotMatch(composerSection, /Plan skill is unavailable/);
});

test('slash skill selection stays in the composer instead of submitting immediately', () => {
  const source = read(conversationComposerDockPath);
  const slashHandler = source.slice(
    source.indexOf('const handleSlashItemSelect = useCallback'),
    source.indexOf('const handleGlobalPlanSkillHotkey = useCallback'),
  );

  assert.match(slashHandler, /applySelectedSkillOption\(item,\s*\{\s*draft: nextDraft,/m);
  assert.doesNotMatch(slashHandler, /void submitChatTurn\(\{ selectedSkillOverride \}\)/);
});
