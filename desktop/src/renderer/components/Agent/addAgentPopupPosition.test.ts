import assert from 'node:assert/strict';
import test from 'node:test';

const { getAddAgentPopupPosition } =
  // @ts-ignore Node strip-types test runner requires explicit .ts extensions here.
  await import('./addAgentPopupPosition.ts');

test('positions point anchors at the click point with cursor offset', () => {
  assert.deepEqual(
    getAddAgentPopupPosition(
      { kind: 'point', x: 100, y: 80 },
      { width: 220, height: 160 },
      { width: 800, height: 600 },
    ),
    { left: 108, top: 80 },
  );
});

test('positions rect anchors beside the trigger', () => {
  assert.deepEqual(
    getAddAgentPopupPosition(
      { kind: 'rect', rect: { left: 20, top: 30, right: 44, bottom: 54 } },
      { width: 220, height: 160 },
      { width: 800, height: 600 },
    ),
    { left: 48, top: 30 },
  );
});

test('flips point anchors near the right and bottom viewport edges', () => {
  assert.deepEqual(
    getAddAgentPopupPosition(
      { kind: 'point', x: 490, y: 390 },
      { width: 220, height: 160 },
      { width: 500, height: 400 },
    ),
    { left: 262, top: 222 },
  );
});

test('flips rect anchors near the right and bottom viewport edges', () => {
  assert.deepEqual(
    getAddAgentPopupPosition(
      { kind: 'rect', rect: { left: 470, top: 300, right: 494, bottom: 324 } },
      { width: 220, height: 160 },
      { width: 500, height: 400 },
    ),
    { left: 246, top: 164 },
  );
});

test('clamps rect anchors when the trigger itself is outside the viewport edge', () => {
  assert.deepEqual(
    getAddAgentPopupPosition(
      { kind: 'rect', rect: { left: 470, top: 380, right: 494, bottom: 404 } },
      { width: 220, height: 160 },
      { width: 500, height: 400 },
    ),
    { left: 246, top: 232 },
  );
});

test('keeps popup origin inside the viewport margin when viewport is smaller than the popup', () => {
  assert.deepEqual(
    getAddAgentPopupPosition(
      { kind: 'point', x: 1, y: 1 },
      { width: 300, height: 200 },
      { width: 180, height: 120 },
    ),
    { left: 8, top: 8 },
  );
});
