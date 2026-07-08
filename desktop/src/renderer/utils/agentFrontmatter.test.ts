import assert from 'node:assert/strict';
import test from 'node:test';

const {
  buildReferenceAgentMarkdown,
  normalizeAgentNodeID,
} =
  // @ts-ignore Node strip-types test runner requires explicit .ts extensions here.
  await import('./agentFrontmatter.ts');

test('buildReferenceAgentMarkdown writes node id bind references only', () => {
  assert.equal(normalizeAgentNodeID('@agent-demo'), 'agent-demo');
  assert.equal(buildReferenceAgentMarkdown('agent-demo'), '---\nbind: @agent-demo\n---\n');
});

test('buildReferenceAgentMarkdown rejects legacy node keys', () => {
  assert.equal(
    normalizeAgentNodeID('@local:host:agent:file:///tmp/.agent/AGENT.md'),
    '',
  );
  assert.throws(
    () => buildReferenceAgentMarkdown('@local:host:agent:file:///tmp/.agent/AGENT.md'),
    /agent node id is required/,
  );
});
