import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const widgetPath = path.join(__dirname, 'ChatHeaderWidget.ts');
const stylesPath = path.join(__dirname, '../../../../styles/index.css');

test('ChatHeaderWidget uses static glass capsule and shared avatar initials', () => {
  const source = readFileSync(widgetPath, 'utf8');

  assert.match(source, /OP_SG_CAPSULE/);
  assert.match(source, /OP_SG_CAPSULE_ON_EDITOR/);
  assert.match(source, /from '\.\.\/\.\.\/\.\.\/avatarInitials'/);
  assert.match(source, /resolveUserAvatarSrc/);
  assert.match(source, /createUserAvatarIconElement/);
  assert.match(source, /showGuestUserAvatar/);
  assert.match(source, /applyCurrentUserAvatar/);
  assert.match(source, /buildInitials/);
  assert.match(source, /initialsBackgroundColor/);
  assert.match(source, /cm-md-chat-avatar/);
  assert.doesNotMatch(source, /FALLBACK_PALETTE/);
  assert.doesNotMatch(source, /function fnv1a/);
  assert.doesNotMatch(source, /backdrop-blur/);
  assert.doesNotMatch(source, /op-sg-card/);
});

test('ChatHeaderWidget uses the shared current-user avatar resolver', () => {
  const source = readFileSync(widgetPath, 'utf8');

  assert.match(source, /resolveUserAvatarSrc\(profile\)/);
  assert.doesNotMatch(source, /localAvatar/);
});

test('chat avatar styles use 20px capsule sizing aligned with titlebar avatar', () => {
  const source = readFileSync(stylesPath, 'utf8');

  assert.match(source, /\.cm-md-chat-avatar\s*\{[^}]*width:\s*20px;/m);
  assert.match(source, /\.cm-md-chat-avatar\s*\{[^}]*height:\s*20px;/m);
  assert.match(source, /\.cm-md-chat-avatar\.op-sg-capsule\s*\{/);
  assert.match(source, /\.cm-md-chat-avatar-img,\s*\n\.cm-md-chat-avatar-fallback,\s*\n\.cm-md-chat-avatar-icon\s*\{[^}]*position:\s*absolute;/m);
  assert.match(source, /\.cm-md-chat-avatar-icon svg\s*\{[^}]*width:\s*12px;/m);
  assert.match(source, /\.cm-md-chat-avatar-fallback\s*\{[^}]*font-size:\s*9px;/m);
  assert.match(source, /\.cm-md-chat-header\s*\{[^}]*gap:\s*6px;/m);
});
