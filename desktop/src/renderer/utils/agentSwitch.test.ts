import assert from 'node:assert/strict';
import test from 'node:test';

const { resolveAgentRootWorkdir } =
  // @ts-ignore Node strip-types test runner requires explicit .ts extensions here.
  await import('./agentSwitch.ts');

test('resolveAgentRootWorkdir keeps ordinary directories unchanged', () => {
  assert.equal(resolveAgentRootWorkdir('/workspace'), '/workspace');
  assert.equal(resolveAgentRootWorkdir('/workspace/src'), '/workspace/src');
});

test('resolveAgentRootWorkdir maps .agent resource dirs to agent root', () => {
  assert.equal(resolveAgentRootWorkdir('/workspace/.agent'), '/workspace');
  assert.equal(resolveAgentRootWorkdir('/workspace/.agent/chat'), '/workspace');
  assert.equal(resolveAgentRootWorkdir('/workspace/.agent/assets'), '/workspace');
});

test('resolveAgentRootWorkdir resolves nested subagent resource dirs', () => {
  assert.equal(
    resolveAgentRootWorkdir('/workspace/.agent/subagents/helper/.agent'),
    '/workspace/.agent/subagents/helper',
  );
  assert.equal(
    resolveAgentRootWorkdir('/workspace/.agent/subagents/helper/.agent/chat'),
    '/workspace/.agent/subagents/helper',
  );
});

test('resolveAgentRootWorkdir handles root-level .agent edge case', () => {
  assert.equal(resolveAgentRootWorkdir('/.agent'), '/');
  assert.equal(resolveAgentRootWorkdir('/.agent/chat'), '/');
});
