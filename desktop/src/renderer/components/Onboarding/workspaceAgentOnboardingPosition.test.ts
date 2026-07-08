import assert from 'node:assert/strict';
import test from 'node:test';

import { getWorkspaceAgentOnboardingPosition } from './workspaceAgentOnboardingPosition.ts';

const card = { width: 288, height: 138 };
const viewport = { width: 1000, height: 720 };

test('workspace agent onboarding card prefers the right side when there is room', () => {
  const position = getWorkspaceAgentOnboardingPosition(
    { left: 240, top: 120, right: 320, bottom: 152, width: 80, height: 32 },
    card,
    viewport,
  );

  assert.equal(position.placement, 'right');
  assert.equal(position.left, 332);
});

test('workspace agent onboarding card falls back to the left side near the right edge', () => {
  const position = getWorkspaceAgentOnboardingPosition(
    { left: 850, top: 120, right: 930, bottom: 152, width: 80, height: 32 },
    card,
    viewport,
  );

  assert.equal(position.placement, 'left');
  assert.equal(position.left, 550);
});

test('workspace agent onboarding card uses vertical placement and clamps to viewport', () => {
  const position = getWorkspaceAgentOnboardingPosition(
    { left: 16, top: 40, right: 960, bottom: 72, width: 944, height: 32 },
    card,
    viewport,
  );

  assert.equal(position.placement, 'bottom');
  assert.ok(position.left >= 8);
  assert.ok(position.left + card.width <= viewport.width - 8);
});
