import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const avatarPath = path.join(__dirname, 'TitlebarUserAvatar.tsx');
const appPath = path.join(__dirname, '../App.tsx');
const logoMenuPath = path.join(__dirname, 'TitlebarLogoMenu.tsx');
const stylesPath = path.join(__dirname, '../styles/index.css');

test('TitlebarUserAvatar uses static glass capsule on titlebar for all states', () => {
  const source = readFileSync(avatarPath, 'utf8');

  assert.match(source, /OP_SG_CAPSULE/);
  assert.match(source, /OP_SG_CAPSULE_ON_TITLEBAR/);
  assert.match(source, /OP_SG_CAPSULE_ON_EDITOR/);
  assert.match(source, /titlebar-user-avatar/);
  assert.match(source, /UserIcon className="titlebar-user-avatar__icon"/);
  assert.match(source, /titlebar-user-avatar__initials/);
  assert.match(source, /titlebar-user-avatar__img/);
  assert.doesNotMatch(source, /bg-accent/);
  assert.doesNotMatch(source, /backdrop-blur/);
  assert.doesNotMatch(source, /op-sg-card/);
});

test('TitlebarUserAvatar uses only profile avatar for signed-in users', () => {
  const source = readFileSync(avatarPath, 'utf8');

  assert.match(source, /profile\?\.avatar/);
  assert.doesNotMatch(source, /localAvatar/);
});

test('TitlebarUserAvatar renders avatar source directly from profile', () => {
  const source = readFileSync(avatarPath, 'utf8');

  assert.match(source, /src=\{avatarSrc\}/);
  assert.doesNotMatch(source, /file:\/\//);
  assert.doesNotMatch(source, /resolveAvatarImageSrc/);
});

test('TitlebarUserAvatar logged-out state does not render initials fallback', () => {
  const source = readFileSync(avatarPath, 'utf8');

  assert.match(source, /if \(!loggedIn\)/);
  const loggedOutBlock = source.slice(
    source.indexOf('if (!loggedIn)'),
    source.indexOf('if (avatarSrc && !imageFailed)'),
  );
  assert.doesNotMatch(loggedOutBlock, /buildInitials/);
  assert.doesNotMatch(loggedOutBlock, /initialsBackgroundColor/);
});

test('App.tsx wires titlebar menu and TitlebarLogoMenu renders user avatar in titlebar and menu', () => {
  const appSource = readFileSync(appPath, 'utf8');
  const menuSource = readFileSync(logoMenuPath, 'utf8');

  assert.match(appSource, /import \{ TitlebarLogoMenu \} from '\.\/components\/TitlebarLogoMenu';/);
  assert.match(appSource, /<TitlebarLogoMenu/);
  assert.match(menuSource, /import \{ TitlebarUserAvatar \} from '\.\/TitlebarUserAvatar';/);
  assert.doesNotMatch(menuSource, /HomeIcon/);
  assert.match(menuSource, /size="titlebar"/);
  assert.match(menuSource, /size="menu"/);
  assert.doesNotMatch(menuSource, /bg-accent text-white flex items-center justify-center text-xs font-medium flex-shrink-0/);
});

test('titlebar user avatar styles use capsule sizing tokens', () => {
  const source = readFileSync(stylesPath, 'utf8');

  assert.match(source, /\.titlebar-user-avatar\.op-sg-capsule\s*\{[^}]*width:\s*20px;/m);
  assert.match(source, /\.titlebar-user-avatar--menu\.op-sg-capsule\s*\{[^}]*width:\s*24px;/m);
  assert.match(source, /\.titlebar-user-avatar--profile\.op-sg-capsule\s*\{[^}]*width:\s*48px;/m);
  assert.match(source, /\.titlebar-user-avatar__initials\s*\{/);
});
