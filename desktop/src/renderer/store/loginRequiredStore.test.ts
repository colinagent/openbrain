import assert from 'node:assert/strict';
import test from 'node:test';

import { showLoginRequiredDialog, useLoginRequiredStore } from './loginRequiredStore.ts';

test('login required dialog store opens with an explicit reason and can hide', () => {
  useLoginRequiredStore.setState({ open: false, reason: 'chat' });

  useLoginRequiredStore.getState().show('resume');
  assert.equal(useLoginRequiredStore.getState().open, true);
  assert.equal(useLoginRequiredStore.getState().reason, 'resume');

  useLoginRequiredStore.getState().hide();
  assert.equal(useLoginRequiredStore.getState().open, false);
});

test('login required helper opens the shared dialog store', () => {
  useLoginRequiredStore.setState({ open: false, reason: 'chat' });

  showLoginRequiredDialog('thread-control');

  assert.equal(useLoginRequiredStore.getState().open, true);
  assert.equal(useLoginRequiredStore.getState().reason, 'thread-control');
});
