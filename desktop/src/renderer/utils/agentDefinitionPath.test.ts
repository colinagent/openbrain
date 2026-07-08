import assert from 'node:assert/strict';
import test from 'node:test';

import { resolveAgentDefinitionPath } from './agentDefinitionPath';

test('resolveAgentDefinitionPath prefers agent definition URI', () => {
  const path = resolveAgentDefinitionPath(
    {
      uri: 'file:///tmp/workspace/agents/coder/.agent/AGENT.md',
      cwd: '/tmp/other',
    } as never,
    null,
  );
  assert.equal(path, '/tmp/workspace/agents/coder/.agent/AGENT.md');
});

test('resolveAgentDefinitionPath falls back to cwd agent definition', () => {
  const path = resolveAgentDefinitionPath(
    { cwd: '/tmp/workspace/agents/coder' } as never,
    null,
  );
  assert.equal(path, '/tmp/workspace/agents/coder/.agent/AGENT.md');
});
