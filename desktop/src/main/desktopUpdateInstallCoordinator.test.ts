import assert from 'node:assert/strict';
import test from 'node:test';

import { DesktopUpdateInstallCoordinator } from './desktopUpdateInstallCoordinator';

test('DesktopUpdateInstallCoordinator tracks awaiting windows and skips already-pending closes', () => {
  const coordinator = new DesktopUpdateInstallCoordinator();
  const plan = coordinator.planInstall([11, 12, 13], [12]);

  assert.deepEqual(plan.awaitingWindowIds, [11, 12, 13]);
  assert.deepEqual(plan.requestCloseWindowIds, [11, 13]);
  assert.equal(plan.shouldInstallImmediately, false);
  assert.equal(coordinator.isActive(), true);
});

test('DesktopUpdateInstallCoordinator only finalizes after the last awaited window closes', () => {
  const coordinator = new DesktopUpdateInstallCoordinator();
  coordinator.planInstall([21, 22], []);

  assert.equal(coordinator.markWindowClosed(999), false);
  assert.equal(coordinator.markWindowClosed(21), false);
  assert.equal(coordinator.markWindowClosed(22), true);
  assert.equal(coordinator.isActive(), false);
});

test('DesktopUpdateInstallCoordinator installs immediately when no windows remain', () => {
  const coordinator = new DesktopUpdateInstallCoordinator();
  const plan = coordinator.planInstall([], []);

  assert.deepEqual(plan.awaitingWindowIds, []);
  assert.deepEqual(plan.requestCloseWindowIds, []);
  assert.equal(plan.shouldInstallImmediately, true);
  assert.equal(coordinator.isActive(), false);
});
