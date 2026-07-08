import assert from 'node:assert/strict';
import test from 'node:test';

import type { OpNode } from '../../services/agentService';
import {
  PRIMARY_CHAT_CAPABLE_AGENT_OPCODE,
  buildAgentSwitchOptions,
  findChatCapableAgentOpcode,
  formatAgentTargetDisplayLabel,
  formatAgentTargetDisplayTitle,
  isChatCapableAgentNode,
} from './agentSwitchOptions';

function createAgentNode(input: Partial<OpNode> & Pick<OpNode, 'id'>): OpNode {
  return {
    id: input.id,
    uid: input.uid || 'user-1',
    kind: input.kind || 'agent',
    uri: input.uri || '',
    cwd: input.cwd,
    tags: input.tags,
    opCodes: input.opCodes,
    run: input.run,
    meta: input.meta,
  };
}

test('buildAgentSwitchOptions keeps only global non-bind agents and marks the current selection', () => {
  const options = buildAgentSwitchOptions({
    agentsRootDir: '/root/.openbrain/agents',
    currentAgentID: 'agent-beta',
    agentNodes: [
      createAgentNode({
        id: 'agent-beta',
        cwd: '/root/.openbrain/agents/beta',
        opCodes: [PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: { name: 'Beta' },
      }),
      createAgentNode({
        id: 'agent-alpha',
        cwd: '/root/.openbrain/agents/alpha',
        opCodes: [PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: { name: 'Alpha' },
      }),
      createAgentNode({
        id: 'agent-alpha',
        cwd: '/root/.openbrain/agents/alpha-copy',
        opCodes: [PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: { name: 'Alpha Duplicate' },
      }),
      createAgentNode({
        id: 'agent-bound',
        cwd: '/root/.openbrain/agents/bound',
        opCodes: [PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: { name: 'Bound', bind: '@agent-alpha' },
      }),
      createAgentNode({
        id: 'agent-custom',
        cwd: '/root/.openbrain/workspace/demo',
        opCodes: [PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: { name: 'Workspace Custom' },
      }),
      createAgentNode({
        id: 'skill-1',
        kind: 'skill',
        cwd: '/root/.openbrain/agents/skill',
        meta: { name: 'Skill' },
      }),
    ],
  });

  assert.deepEqual(options, [
    {
      id: 'agent-alpha',
      name: 'Alpha',
      path: '/root/.openbrain/agents/alpha',
      selected: false,
    },
    {
      id: 'agent-beta',
      name: 'Beta',
      path: '/root/.openbrain/agents/beta',
      selected: true,
    },
  ]);
});

test('buildAgentSwitchOptions falls back to id, normalizes @currentAgentID, and sorts by path when names tie', () => {
  const options = buildAgentSwitchOptions({
    agentsRootDir: '/root/.openbrain/agents',
    currentAgentID: '@agent-b',
    agentNodes: [
      createAgentNode({
        id: 'agent-b',
        uri: 'file:///root/.openbrain/agents/team-b/.agent/AGENT.md',
        opCodes: ['notify/message', PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: {},
      }),
      createAgentNode({
        id: 'agent-a',
        uri: 'file:///root/.openbrain/agents/team-a/.agent/AGENT.md',
        opCodes: [PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
        meta: {},
      }),
    ],
  });

  assert.deepEqual(options, [
    {
      id: 'agent-a',
      name: 'agent-a',
      path: '/root/.openbrain/agents/team-a',
      selected: false,
    },
    {
      id: 'agent-b',
      name: 'agent-b',
      path: '/root/.openbrain/agents/team-b',
      selected: true,
    },
  ]);
});

test('formatAgentTargetDisplayLabel and title separate workspace and agent without looking like a path', () => {
  assert.equal(
    formatAgentTargetDisplayLabel('/Users/example/code/sample-workspace', 'openbrain'),
    'sample-workspace:openbrain',
  );
  assert.equal(
    formatAgentTargetDisplayLabel('', 'gbrain'),
    'workspace:gbrain',
  );
  assert.equal(
    formatAgentTargetDisplayLabel('/Users/example/code/sample-workspace', null),
    'sample-workspace:—',
  );
  assert.equal(
    formatAgentTargetDisplayTitle('/Users/example/code/sample-workspace', 'openbrain'),
    '/Users/example/code/sample-workspace · openbrain',
  );
  assert.equal(
    formatAgentTargetDisplayTitle('', 'gbrain'),
    'workspace · gbrain',
  );
});

test('chat-capable helpers only accept thread/submit', () => {
  const primaryChatCapable = createAgentNode({
    id: 'agent-ok-primary',
    cwd: '/root/.openbrain/agents/ok-primary',
    opCodes: ['notify/message', PRIMARY_CHAT_CAPABLE_AGENT_OPCODE],
  });
  const notCapable = createAgentNode({
    id: 'agent-no',
    cwd: '/root/.openbrain/agents/no',
    opCodes: ['system/started', 'notify/message'],
  });

  assert.equal(findChatCapableAgentOpcode(primaryChatCapable), PRIMARY_CHAT_CAPABLE_AGENT_OPCODE);
  assert.equal(findChatCapableAgentOpcode(notCapable), null);
  assert.equal(isChatCapableAgentNode(primaryChatCapable), true);
  assert.equal(isChatCapableAgentNode(notCapable), false);
});
