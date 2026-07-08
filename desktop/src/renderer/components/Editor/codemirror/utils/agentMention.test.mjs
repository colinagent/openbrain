import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./agentMention.ts', import.meta.url);
const sourceText = await readFile(sourcePath, 'utf8');
const transpiled = ts.transpileModule(sourceText, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const sourceModule = await import(
  `data:text/javascript;base64,${Buffer.from(transpiled, 'utf8').toString('base64')}`
);
const {
  buildAgentLinkTarget,
  parseAgentLinkTarget,
  parseAgentMentionValue,
  parseAgentMentionsInText,
} = sourceModule;

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
