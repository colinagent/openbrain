import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getHeadingMarkerInfo,
  getListMarkerSourceClassName,
  parseChatParticipantMarkerLine,
} from './livePreviewPlugin';
import {
  buildAgentLinkTarget,
  parseAgentLinkTarget,
  parseAgentMentionValue,
  parseAgentMentionsInText,
} from './utils/agentMention';

function createMockView(doc: string) {
  return {
    state: {
      sliceDoc(from: number, to: number) {
        return doc.slice(from, to);
      },
    },
  };
}

test('heading marker info keeps the whitespace after # outside the marker span', () => {
  const doc = '## heading';
  const info = getHeadingMarkerInfo(createMockView(doc), 0, doc.length);

  assert.deepEqual(info, {
    hashesTo: 2,
    contentFrom: 3,
  });
});

test('heading marker info supports multiple spaces and tabs before heading content', () => {
  const doc = '### \t heading';
  const info = getHeadingMarkerInfo(createMockView(doc), 0, doc.length);

  assert.deepEqual(info, {
    hashesTo: 3,
    contentFrom: 6,
  });
});

test('heading marker info returns null for non-heading text', () => {
  const doc = 'plain text';
  assert.equal(getHeadingMarkerInfo(createMockView(doc), 0, doc.length), null);
});

test('list marker source class is source-only when inactive', () => {
  assert.equal(getListMarkerSourceClassName(false), 'cm-md-list-marker-source');
});

test('list marker source class includes shared syntax-visible semantics when active', () => {
  assert.equal(
    getListMarkerSourceClassName(true),
    'cm-md-list-marker-source cm-md-list-marker-source-active cm-md-syntax-visible'
  );
});

test('parseChatParticipantMarkerLine accepts exclusive @user and @agent lines', () => {
  assert.deepEqual(parseChatParticipantMarkerLine('  @user-example  '), {
    role: 'user',
    id: 'user-example',
    from: 2,
    to: 13,
  });
  assert.deepEqual(parseChatParticipantMarkerLine('@agent-coder'), {
    role: 'agent',
    id: 'agent-coder',
    from: 0,
    to: 12,
  });
});

test('parseChatParticipantMarkerLine rejects inline mentions', () => {
  assert.equal(parseChatParticipantMarkerLine('hello @agent-coder'), null);
  assert.equal(parseChatParticipantMarkerLine('@agent-coder hello'), null);
});

test('parseAgentMentionsInText accepts inline agent IDs with markdown-like boundaries', () => {
  assert.deepEqual(parseAgentMentionsInText('bind: @agent-d7m8'), [
    { agentID: 'agent-d7m8', from: 6, to: 17 },
  ]);
  assert.deepEqual(parseAgentMentionsInText('hello @agent-coder.'), [
    { agentID: 'agent-coder', from: 6, to: 18 },
  ]);
});

test('parseAgentMentionsInText rejects legacy IDs and embedded address-like text', () => {
  assert.deepEqual(parseAgentMentionsInText('@local:host:agent:file:///tmp/.agent/AGENT.md'), []);
  assert.deepEqual(parseAgentMentionsInText('@skill-plan'), []);
  assert.deepEqual(parseAgentMentionsInText('foo@agent-coder'), []);
});

test('agent link targets round-trip valid agent IDs only', () => {
  assert.equal(buildAgentLinkTarget('agent-coder'), 'agent:agent-coder');
  assert.equal(buildAgentLinkTarget('@agent-coder'), 'agent:agent-coder');
  assert.equal(parseAgentLinkTarget('agent:agent-coder'), 'agent-coder');
  assert.equal(parseAgentLinkTarget('thread:thread-demo'), null);
  assert.equal(buildAgentLinkTarget('@local:host:agent:file:///tmp/.agent/AGENT.md'), null);
});

test('parseAgentMentionValue requires a single explicit @agent ID', () => {
  assert.equal(parseAgentMentionValue('@agent-coder'), 'agent-coder');
  assert.equal(parseAgentMentionValue('agent-coder'), null);
  assert.equal(parseAgentMentionValue('@agent-coder extra'), null);
});
