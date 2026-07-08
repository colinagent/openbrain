import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './OpenBrainSidebar.tsx'),
  'utf8',
);

test('OpenBrainSidebar places actions beside the OpenBrain provider label', () => {
  assert.match(
    source,
    /<div className="ui-tabbar sidebar-root-header openbrain-sidebar-header flex shrink-0 items-center gap-1 overflow-hidden px-2 text-secondary-text">[\s\S]*<span className="ui-chrome-row-label truncate">OpenBrain · \{provider === 'local' \? 'Local' : 'Cloud'\}<\/span>[\s\S]*className="sidebar-root-header-actions ml-auto flex shrink-0 items-center gap-0\.5"[\s\S]*aria-label="Create OpenBrain source"[\s\S]*aria-label="Refresh OpenBrain sources"[\s\S]*aria-label="OpenBrain settings"/,
  );
  assert.match(source, /useAppStore\(\(state\) => state\.openOpenBrainSettingsTab\)/);
  assert.match(source, /useAuthStore\(\(state\) => state\.startLogin\)/);
  assert.match(source, /useUiStore\.getState\(\)\.setSidebarView\('workspace'\);[\s\S]*openOpenBrainSettingsTab\(\);/);
  assert.match(source, /authRequired && !loggedIn[\s\S]*className="[^"]*underline[\s\S]*onClick=\{\(\) => void handleSignIn\(\)\}/);
  assert.match(source, /authRequired && loggedIn[\s\S]*OpenBrain Cloud is not available for this account\./);
  assert.match(source, /const authRevision = useAuthStore\(\(state\) => state\.authRevision\)/);
  assert.match(source, /postLoginRefreshRevisionRef[\s\S]*postLoginRefreshRevisionRef\.current === authRevision[\s\S]*postLoginRefreshRevisionRef\.current = authRevision[\s\S]*void refresh\(\)\.catch/);
  assert.match(source, /const deviceCodeError = useAuthStore\(\(state\) => state\.deviceCodeError\)/);
  assert.match(source, /lastDeviceCodeErrorRef[\s\S]*pushToast\(deviceCodeError\)/);
  assert.doesNotMatch(source, /variant="inline"/);
  assert.match(source, /<PlusIcon className="h-3\.5 w-3\.5" \/>/);
  assert.match(source, /<SettingsIcon className="h-3\.5 w-3\.5" \/>/);
  assert.doesNotMatch(source, /<div className="ui-tabbar flex shrink-0 items-center justify-center overflow-hidden px-2">/);
});
