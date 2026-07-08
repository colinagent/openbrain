import test from 'node:test';
import assert from 'node:assert/strict';

import { removeSlashTokenFromDraft, resolveSlashMenuState } from './chatSlash.ts';

const BUILT_INS = [
  { key: 'builtin-compact', slug: 'compact', name: 'Compact', description: 'Compact the current thread context' },
];

const SKILLS = [
  { id: 'skill-plan', slug: 'plan', name: 'Plan', description: 'Maintain plan files' },
  { id: 'skill-execute-plan', slug: 'execute-plan', name: 'Execute Plan', description: 'Run the plan' },
];

function resolveState(overrides = {}) {
  return resolveSlashMenuState({
    draft: '',
    cursorPos: null,
    isCommandMode: false,
    isQueuedReadOnly: false,
    dismissedSlashToken: null,
    skillOptions: SKILLS,
    builtInCommands: BUILT_INS,
    agentNodesLoading: false,
    ...overrides,
  });
}

test('returns loading when slash is active and skill nodes are still loading', () => {
  const state = resolveState({
    draft: '/pl',
    cursorPos: 3,
    skillOptions: [],
    agentNodesLoading: true,
  });

  assert.equal(state.status, 'loading');
});

test('returns no-commands when slash is active and there are no skill nodes after loading', () => {
  const state = resolveState({
    draft: '/pl',
    cursorPos: 3,
    skillOptions: [],
    agentNodesLoading: false,
  });

  assert.equal(state.status, 'no-commands');
});

test('returns no-match when skill nodes exist but none match the query', () => {
  const state = resolveState({
    draft: '/unknown',
    cursorPos: 8,
  });

  assert.equal(state.status, 'no-match');
});

test('returns results when the query matches a skill slug', () => {
  const state = resolveState({
    draft: '/pl',
    cursorPos: 3,
  });

  assert.equal(state.status, 'results');
  assert.deepEqual(state.filteredSkillOptions.map((option) => option.slug), ['plan', 'execute-plan']);
});

test('returns builtin command results before skills', () => {
  const state = resolveState({
    draft: '/co',
    cursorPos: 3,
  });

  assert.equal(state.status, 'results');
  assert.deepEqual(state.filteredItems.map((item) => `${item.kind}:${item.slug}`), ['command:compact']);
});

test('returns results when the slash token appears after earlier text', () => {
  const state = resolveState({
    draft: 'hello /pl',
    cursorPos: 9,
  });

  assert.equal(state.status, 'results');
  assert.deepEqual(state.filteredSkillOptions.map((option) => option.slug), ['plan', 'execute-plan']);
});

test('keeps slash hidden when the first token contains a second slash', () => {
  const state = resolveState({
    draft: '/pl/extra',
    cursorPos: 9,
  });

  assert.equal(state.status, 'hidden');
});

test('keeps slash hidden when the cursor is not inside the slash token', () => {
  const state = resolveState({
    draft: 'hello /pl world',
    cursorPos: 2,
  });

  assert.equal(state.status, 'hidden');
});

test('removes the slash token in place without clobbering surrounding text', () => {
  const state = resolveState({
    draft: 'hello /pl world',
    cursorPos: 9,
  });

  assert.equal(state.status, 'results');
  assert.equal(removeSlashTokenFromDraft('hello /pl world', state.slashState), 'hello world');
});
