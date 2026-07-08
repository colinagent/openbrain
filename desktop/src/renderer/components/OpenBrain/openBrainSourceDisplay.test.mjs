import assert from 'node:assert/strict';
import test from 'node:test';

import { resolveOpenBrainSourceDisplayState } from './openBrainSourceDisplay.ts';

/** @param {Record<string, unknown>} overrides */
function baseSource(overrides = {}) {
  return {
    sourceID: 'src-1',
    name: 'note',
    openable: true,
    bindingStatus: 'connected',
    path: '/Users/example/code/note',
    ...overrides,
  };
}

/** @param {ReturnType<typeof baseSource>} source */
function cloudDisplay(source, uiLinked = true) {
  return resolveOpenBrainSourceDisplayState(source, { provider: 'cloud', uiLinked });
}

/** @param {ReturnType<typeof baseSource>} source */
function localDisplay(source, uiLinked = true) {
  return resolveOpenBrainSourceDisplayState(source, { provider: 'local', uiLinked });
}

test('resolveOpenBrainSourceDisplayState statusText priority', () => {
  assert.equal(
    cloudDisplay(baseSource({ runtimeReachable: false })).statusText,
    'Runtime offline',
  );
  assert.equal(
    cloudDisplay(baseSource({ bindingStatus: 'needs_binding', bindingReason: 'moved' })).statusText,
    'Folder moved',
  );
  assert.equal(
    cloudDisplay(baseSource({ bindingStatus: 'needs_binding', bindingReason: 'mismatch' })).statusText,
    'Repo mismatch',
  );
  assert.equal(
    cloudDisplay(baseSource({ bindingStatus: 'needs_binding', bindingReason: 'unbound' })).statusText,
    'Needs binding',
  );
  assert.equal(
    cloudDisplay(baseSource({ disabledQueries: true })).statusText,
    'Disabled query',
  );
  assert.equal(
    cloudDisplay(baseSource({ disabledQueries: true, publicAccess: true })).statusText,
    'Disabled query',
  );
  assert.equal(
    cloudDisplay(baseSource({ publicAccess: true })).statusText,
    'Public',
  );
  assert.equal(
    cloudDisplay(baseSource()).statusText,
    'Connected',
  );
  assert.equal(
    cloudDisplay(baseSource({ bindingStatus: 'needs_binding', disabledQueries: true })).statusText,
    'Needs binding',
  );
});

test('resolveOpenBrainSourceDisplayState arcLinked is not gated by binding or runtime', () => {
  assert.equal(
    cloudDisplay(baseSource({ runtimeReachable: false })).arcLinked,
    true,
  );
  assert.equal(
    cloudDisplay(baseSource({ bindingStatus: 'needs_binding', bindingReason: 'moved' })).arcLinked,
    true,
  );
  assert.equal(cloudDisplay(baseSource(), false).arcLinked, false);
  assert.equal(
    cloudDisplay(baseSource({ disabledQueries: true })).arcLinked,
    false,
  );
  assert.equal(
    cloudDisplay(baseSource({ bindingStatus: 'needs_binding', disabledQueries: true })).arcLinked,
    false,
  );
  assert.equal(localDisplay(baseSource(), true).arcLinked, true);
  assert.equal(localDisplay(baseSource(), false).arcLinked, false);
  assert.equal(localDisplay(baseSource(), false).statusText, 'Connected');
});

test('resolveOpenBrainSourceDisplayState detail and openable', () => {
  const connected = cloudDisplay(baseSource());
  assert.equal(connected.detail, '/Users/example/code/note');
  assert.equal(connected.openable, true);

  const needsBinding = cloudDisplay(baseSource({ bindingStatus: 'needs_binding', path: undefined }));
  assert.equal(needsBinding.detail, undefined);
  assert.equal(needsBinding.openable, false);

  const offline = cloudDisplay(baseSource({ runtimeReachable: false }));
  assert.equal(offline.openable, false);
});

test('resolveOpenBrainSourceDisplayState menu flags', () => {
  const connected = cloudDisplay(baseSource());
  assert.equal(connected.menu.canOpen, true);
  assert.equal(connected.menu.canBind, false);
  assert.equal(connected.menu.canRemoveFromDevice, true);
  assert.equal(connected.menu.canManageCloud, true);

  const needsBinding = cloudDisplay(baseSource({ bindingStatus: 'needs_binding', path: undefined }));
  assert.equal(needsBinding.menu.canBind, true);
  assert.equal(needsBinding.menu.canOpen, false);

  const offline = cloudDisplay(baseSource({ runtimeReachable: false }));
  assert.equal(offline.menu.canBind, false);
  assert.equal(offline.menu.canRemoveFromDevice, false);
  assert.equal(offline.menu.canManageCloud, true);

  const granted = cloudDisplay(baseSource({ bindingMode: 'granted' }));
  assert.equal(granted.menu.canManageCloud, false);
});
