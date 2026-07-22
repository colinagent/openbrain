import assert from 'node:assert/strict';
import test from 'node:test';

import { formatStatusBarPathDisplay } from './statusBarPath';

const workspaceDir = '/Users/example/.openbrain/workspace';
const agentFile = '/Users/example/.openbrain/agents/coder/.agent/AGENT.md';

test('formatStatusBarPathDisplay shows workspace when no file is active', () => {
  assert.deepEqual(formatStatusBarPathDisplay(workspaceDir, null), {
    label: workspaceDir,
  });
  assert.deepEqual(formatStatusBarPathDisplay(workspaceDir, ''), {
    label: workspaceDir,
  });
});

test('formatStatusBarPathDisplay shows No folder when workspace and file are empty', () => {
  assert.deepEqual(formatStatusBarPathDisplay(null, null), {
    label: 'No folder',
  });
});

test('formatStatusBarPathDisplay shows full file path for files inside workspace', () => {
  assert.deepEqual(
    formatStatusBarPathDisplay(workspaceDir, `${workspaceDir}/notes/foo.md`),
    {
      label: `${workspaceDir}/notes/foo.md`,
    },
  );
});

test('formatStatusBarPathDisplay does not duplicate workspace when file equals dir', () => {
  assert.deepEqual(formatStatusBarPathDisplay(workspaceDir, workspaceDir), {
    label: workspaceDir,
  });
});

test('formatStatusBarPathDisplay shows only file path when outside workspace', () => {
  assert.deepEqual(formatStatusBarPathDisplay(workspaceDir, agentFile), {
    label: agentFile,
  });
});

test('formatStatusBarPathDisplay normalizes trailing slashes', () => {
  assert.deepEqual(
    formatStatusBarPathDisplay(`${workspaceDir}/`, `${workspaceDir}/notes/foo.md/`),
    {
      label: `${workspaceDir}/notes/foo.md`,
    },
  );
});
