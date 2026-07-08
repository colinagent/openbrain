import assert from 'node:assert/strict';
import test from 'node:test';

import { DEFAULT_WINDOW_ZOOM_LEVEL, formatWindowZoomPercent } from '../../main/shared/windowZoom';
import {
  applyWindowZoomCommand,
  getCurrentWindowZoomLevel,
  resolveWindowZoomShortcutCommand,
  setCurrentWindowZoomLevel,
  subscribeWindowZoomLevel,
} from './windowZoomShortcuts';

type ShortcutEvent = Parameters<typeof resolveWindowZoomShortcutCommand>[0];

function keyEvent(patch: Partial<ShortcutEvent>): ShortcutEvent {
  return {
    altKey: false,
    code: '',
    ctrlKey: false,
    key: '',
    metaKey: false,
    shiftKey: false,
    ...patch,
  };
}

test('window zoom shortcuts use Cmd on macOS', () => {
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ metaKey: true, code: 'Equal', key: '=' }), 'darwin'),
    'zoomIn'
  );
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'Equal', key: '=' }), 'darwin'),
    null
  );
});

test('window zoom shortcuts use Ctrl outside macOS', () => {
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'Equal', key: '=' }), 'linux'),
    'zoomIn'
  );
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ metaKey: true, code: 'Equal', key: '=' }), 'linux'),
    null
  );
});

test('window zoom shortcuts support plus and numpad add', () => {
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, shiftKey: true, code: 'Equal', key: '+' }), 'linux'),
    'zoomIn'
  );
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'NumpadAdd', key: '+' }), 'linux'),
    'zoomIn'
  );
});

test('window zoom shortcuts support minus and numpad subtract', () => {
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'Minus', key: '-' }), 'linux'),
    'zoomOut'
  );
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'NumpadSubtract', key: '-' }), 'linux'),
    'zoomOut'
  );
});

test('window zoom shortcuts reset on primary modifier plus zero', () => {
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'Digit0', key: '0' }), 'linux'),
    'zoomReset'
  );
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, code: 'Numpad0', key: '0' }), 'linux'),
    'zoomReset'
  );
});

test('window zoom shortcuts ignore alt and plain key presses', () => {
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ ctrlKey: true, altKey: true, code: 'Equal', key: '=' }), 'linux'),
    null
  );
  assert.equal(
    resolveWindowZoomShortcutCommand(keyEvent({ code: 'Equal', key: '=' }), 'linux'),
    null
  );
});

test('window zoom percent follows Electron zoom factor', () => {
  assert.equal(formatWindowZoomPercent(0), '100%');
  assert.equal(formatWindowZoomPercent(1), '120%');
  assert.equal(formatWindowZoomPercent(-1), '83%');
});

test('window zoom commands step by 10 percent and notify listeners', () => {
  setCurrentWindowZoomLevel(DEFAULT_WINDOW_ZOOM_LEVEL);
  const seen: number[] = [];
  const dispose = subscribeWindowZoomLevel((level) => seen.push(level));

  assert.equal(formatWindowZoomPercent(applyWindowZoomCommand('zoomIn')), '110%');
  assert.equal(formatWindowZoomPercent(getCurrentWindowZoomLevel()), '110%');
  assert.equal(applyWindowZoomCommand('zoomReset'), DEFAULT_WINDOW_ZOOM_LEVEL);

  dispose();
  assert.deepEqual(seen.map((level) => formatWindowZoomPercent(level)), ['110%', '100%']);
});
