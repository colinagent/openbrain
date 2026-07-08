import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appSource = readFileSync(path.join(__dirname, 'App.tsx'), 'utf8');
const titlebarMenuSource = readFileSync(path.join(__dirname, 'components/TitlebarLogoMenu.tsx'), 'utf8');

test('titlebar login opens the login dialog instead of direct public sign-in', () => {
  assert.match(
    titlebarMenuSource,
    /const handleLogin = useCallback\(\(\) => \{[\s\S]*showLoginRequiredDialog\('chat'\);[\s\S]*closeMenu\(\);[\s\S]*\}, \[closeMenu\]\);/
  );
  assert.doesNotMatch(appSource, /const authStartLogin = useAuthStore\(\(state\) => state\.startLogin\);/);
});

test('titlebar menu dismisses on outside interaction via shared hook', () => {
  assert.match(titlebarMenuSource, /import \{ useDismissOnOutsideInteraction \} from '\.\.\/hooks\/useDismissOnOutsideInteraction';/);
  assert.match(titlebarMenuSource, /useDismissOnOutsideInteraction\(/);
  assert.doesNotMatch(titlebarMenuSource, /document\.addEventListener\('mousedown', handleOutsideClick\)/);
});

test('App initializes auth listeners before device-code login can be requested', () => {
  assert.match(appSource, /const authInit = useAuthStore\(\(state\) => state\.init\)/);
  assert.match(appSource, /const init = async \(\) => \{[\s\S]*await authInit\(\);[\s\S]*getBootstrap/);
});
