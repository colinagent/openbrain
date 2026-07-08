import assert from 'node:assert/strict';
import test from 'node:test';

import { resolveBootstrapEnsureMode } from './localRuntimeBootstrap';
import {
  MANAGED_SERVER_AGENT_ID,
  buildManagedServerAgentMarkdown,
} from './runtime';

test('resolveBootstrapEnsureMode returns none when runtime is already ready', () => {
  assert.equal(resolveBootstrapEnsureMode({
    needsInstall: false,
    needsUpdate: false,
    needsStart: false,
  }), 'none');
});

test('resolveBootstrapEnsureMode treats start-only recovery as background work', () => {
  assert.equal(resolveBootstrapEnsureMode({
    needsInstall: false,
    needsUpdate: false,
    needsStart: true,
  }), 'background-start');
});

test('resolveBootstrapEnsureMode keeps install and update as blocking work', () => {
  assert.equal(resolveBootstrapEnsureMode({
    needsInstall: true,
    needsUpdate: false,
    needsStart: true,
  }), 'blocking-ensure');
  assert.equal(resolveBootstrapEnsureMode({
    needsInstall: false,
    needsUpdate: true,
    needsStart: true,
  }), 'blocking-ensure');
});

test('buildManagedServerAgentMarkdown includes stable agent id and command', () => {
  const markdown = buildManagedServerAgentMarkdown('./bin/openbrain-server.exe', 19530);
  assert.match(markdown, new RegExp(`^id: ${MANAGED_SERVER_AGENT_ID}$`, 'm'));
  assert.match(markdown, /^name: openbrain-server$/m);
  assert.match(markdown, /\["\.\/bin\/openbrain-server\.exe", "--host", "127\.0\.0\.1", "--port", "19530"\]/);
  assert.match(markdown, /^\s*daemon: true$/m);
});
