import assert from 'node:assert/strict';
import test from 'node:test';

import { useUiStore } from './uiStore';

function resetBlockingModalState() {
  useUiStore.setState({
    blockingModalIds: [],
    hasBlockingModal: false,
  });
}

test('blocking modal registry preserves active state until the last modal closes', (t) => {
  const previousState = useUiStore.getState();
  t.after(() => {
    useUiStore.setState({
      blockingModalIds: previousState.blockingModalIds,
      hasBlockingModal: previousState.hasBlockingModal,
    });
  });
  resetBlockingModalState();

  useUiStore.getState().registerBlockingModal('dialog-a');
  useUiStore.getState().registerBlockingModal('dialog-b');

  assert.deepEqual(useUiStore.getState().blockingModalIds, ['dialog-a', 'dialog-b']);
  assert.equal(useUiStore.getState().hasBlockingModal, true);

  useUiStore.getState().unregisterBlockingModal('dialog-a');

  assert.deepEqual(useUiStore.getState().blockingModalIds, ['dialog-b']);
  assert.equal(useUiStore.getState().hasBlockingModal, true);

  useUiStore.getState().unregisterBlockingModal('dialog-b');

  assert.deepEqual(useUiStore.getState().blockingModalIds, []);
  assert.equal(useUiStore.getState().hasBlockingModal, false);
});

test('blocking modal registry ignores duplicate registrations and unknown removals', (t) => {
  const previousState = useUiStore.getState();
  t.after(() => {
    useUiStore.setState({
      blockingModalIds: previousState.blockingModalIds,
      hasBlockingModal: previousState.hasBlockingModal,
    });
  });
  resetBlockingModalState();

  useUiStore.getState().registerBlockingModal(' dialog-a ');
  useUiStore.getState().registerBlockingModal('dialog-a');
  useUiStore.getState().unregisterBlockingModal('missing-dialog');

  assert.deepEqual(useUiStore.getState().blockingModalIds, ['dialog-a']);
  assert.equal(useUiStore.getState().hasBlockingModal, true);
});

test('chat thinking level store preserves raw service values', (t) => {
  const previousThinkingLevel = useUiStore.getState().chatThinkingLevel;
  t.after(() => {
    useUiStore.getState().setChatThinkingLevel(previousThinkingLevel);
  });

  useUiStore.getState().setChatThinkingLevel(' xhigh ');
  assert.equal(useUiStore.getState().chatThinkingLevel, 'xhigh');
});

test('sidebar view is externally controllable', (t) => {
  const previousView = useUiStore.getState().sidebarView;
  t.after(() => {
    useUiStore.getState().setSidebarView(previousView);
  });

  useUiStore.getState().setSidebarView('openbrain');
  assert.equal(useUiStore.getState().sidebarView, 'openbrain');

  useUiStore.getState().setSidebarView('workspace');
  assert.equal(useUiStore.getState().sidebarView, 'workspace');
});
