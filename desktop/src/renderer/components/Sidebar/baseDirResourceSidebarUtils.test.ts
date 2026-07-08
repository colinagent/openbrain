import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getBaseDirResourceEmptyMessage,
  getBaseDirResourceMissingMessage,
  joinBaseDirResourcePath,
} from './baseDirResourceSidebarUtils';

test('baseDir resource utilities resolve resource roots under the fixed layout', () => {
  assert.equal(joinBaseDirResourcePath('/Users/example/.openbrain', 'skills'), '/Users/example/.openbrain/skills');
  assert.equal(joinBaseDirResourcePath('/Users/example/.openbrain', 'tools'), '/Users/example/.openbrain/tools');
  assert.equal(joinBaseDirResourcePath('/Users/example/.openbrain', 'tasks'), '/Users/example/.openbrain/tasks');
});

test('baseDir resource utilities keep resource-specific empty and missing messages', () => {
  assert.equal(getBaseDirResourceMissingMessage('skills'), 'No skills directory available');
  assert.equal(getBaseDirResourceMissingMessage('tools'), 'No tools directory available');
  assert.equal(getBaseDirResourceMissingMessage('tasks'), 'No tasks directory available');
  assert.equal(getBaseDirResourceEmptyMessage('skills'), 'No skills installed yet');
  assert.equal(getBaseDirResourceEmptyMessage('tools'), 'No tools installed yet');
  assert.equal(getBaseDirResourceEmptyMessage('tasks'), 'No tasks installed yet');
});
