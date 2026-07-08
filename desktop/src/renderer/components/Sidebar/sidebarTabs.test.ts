import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getMainSidebarRailItems,
  isMainSidebarRailItemActive,
  isSidebarMoreRailActive,
} from './sidebarTabs';

test('main sidebar rail keeps Folder, OpenBrain, Messenger, Agents, Skills, and Cron entries', () => {
  assert.deepEqual(
    getMainSidebarRailItems().map((item) => item.label),
    ['Folder', 'OpenBrain', 'Messenger', 'Agents', 'Skills', 'Cron'],
  );
});

test('Cron is a sidebar panel view', () => {
  const items = getMainSidebarRailItems();
  assert.deepEqual(
    items.map((item) => item.key),
    ['workspace', 'openbrain', 'messenger', 'agents', 'skills', 'cron'],
  );
});

test('main sidebar rail active state follows the current sidebar view', () => {
  assert.equal(isMainSidebarRailItemActive('workspace', 'workspace'), true);
  assert.equal(isMainSidebarRailItemActive('openbrain', 'openbrain'), true);
  assert.equal(isMainSidebarRailItemActive('messenger', 'messenger'), true);
  assert.equal(isMainSidebarRailItemActive('agents', 'agents'), true);
  assert.equal(isMainSidebarRailItemActive('skills', 'skills'), true);
  assert.equal(isMainSidebarRailItemActive('cron', 'cron'), true);
  assert.equal(isMainSidebarRailItemActive('workspace', 'agents'), false);
  assert.equal(isMainSidebarRailItemActive('tools', 'workspace'), false);
});

test('sidebar more rail is active for tools or an open menu', () => {
  assert.equal(isSidebarMoreRailActive('tools', false), true);
  assert.equal(isSidebarMoreRailActive('cron', false), false);
  assert.equal(isSidebarMoreRailActive('workspace', true), true);
  assert.equal(isSidebarMoreRailActive('workspace', false), false);
  assert.equal(isSidebarMoreRailActive('search', false), false);
});
