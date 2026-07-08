import assert from 'node:assert/strict';
import test from 'node:test';

import { buildMarketplaceUsageReport } from './marketplaceUsage';
import type { OpNode } from './agentService';

function makeNode(input: Partial<OpNode> & Pick<OpNode, 'id' | 'kind' | 'uri'>): OpNode {
  return {
    id: input.id,
    uid: 'u1',
    kind: input.kind,
    uri: input.uri,
    cwd: input.cwd,
    meta: input.meta,
  };
}

test('buildMarketplaceUsageReport resolves local agent, skill, and tool ids', () => {
  const report = buildMarketplaceUsageReport({
    remote: false,
    baseDir: '/Users/example/.openbrain',
    agentsRootDir: '/Users/example/.openbrain/agents',
    effectiveAgentID: 'agent-openbrain',
    selectedSkillID: 'skill-plan',
    nodes: [
      makeNode({
        id: 'agent-openbrain',
        kind: 'agent',
        uri: 'file:///Users/example/.openbrain/agents/openbrain/.agent/AGENT.md',
        meta: {
          toolServers: ['tools-custom'],
          sysTools: ['read', 'bash'],
        },
      }),
      makeNode({
        id: 'skill-plan',
        kind: 'skill',
        uri: 'file:///Users/example/.openbrain/skills/plan/SKILL.md',
      }),
      makeNode({
        id: 'tools-custom',
        kind: 'tools',
        uri: 'file:///Users/example/.openbrain/tools/custom-tool/TOOL.md',
        meta: {
          name: 'custom-tool',
        },
      }),
    ],
  });

  assert.deepEqual(report.agents, ['openbrain']);
  assert.deepEqual(report.skills, ['plan']);
  assert.deepEqual(report.tools.sort(), ['custom-tool', 'systool']);
});

test('buildMarketplaceUsageReport ignores remote windows and out-of-root resources', () => {
  const report = buildMarketplaceUsageReport({
    remote: true,
    baseDir: '/Users/example/.openbrain',
    agentsRootDir: '/Users/example/.openbrain/agents',
    effectiveAgentID: 'agent-custom',
    selectedSkillID: 'skill-custom',
    nodes: [
      makeNode({
        id: 'agent-custom',
        kind: 'agent',
        uri: 'file:///tmp/custom/.agent/AGENT.md',
        meta: {
          toolServers: ['tools-custom'],
          sysTools: [],
        },
      }),
      makeNode({
        id: 'skill-custom',
        kind: 'skill',
        uri: 'file:///tmp/custom/SKILL.md',
      }),
      makeNode({
        id: 'tools-custom',
        kind: 'tools',
        uri: 'file:///tmp/custom/TOOL.md',
      }),
    ],
  });

  assert.deepEqual(report, { agents: [], skills: [], tools: [] });
});
