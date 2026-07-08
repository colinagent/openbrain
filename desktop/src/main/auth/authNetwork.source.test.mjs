import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const read = (file) => readFileSync(path.join(__dirname, file), 'utf8');
const authSources = [
  'billingStore.ts',
  'deviceCodeAuth.ts',
  'gatewayDiscovery.ts',
  'profileStore.ts',
].map(read).join('\n');
const profileSource = read('profileStore.ts');
const mainSource = readFileSync(path.join(__dirname, '../main.ts'), 'utf8');

test('desktop auth requests use Electron net.fetch instead of Node fetch', () => {
  assert.match(read('netFetch.ts'), /net\.fetch/);
  assert.match(authSources, /authFetch\(/);
  assert.doesNotMatch(authSources, /\bawait fetch\(/);
});

test('device login persists auth even when profile fetch falls back', () => {
  assert.match(mainSource, /saveFallbackProfile/);
  assert.match(mainSource, /await saveAuthConfig\(homeDir, cfg\);[\s\S]*fetchAndSaveProfile\(cfg\.gateway, result\.token\)[\s\S]*saveFallbackProfile\(homeDir, result\.uid, result\.email\)/);
  assert.doesNotMatch(mainSource, /throw new Error\('Failed to fetch profile'\)/);
});

test('device login treats the desktop token exchange as the signed-in boundary', () => {
  const deviceSource = read('deviceCodeAuth.ts');
  assert.match(deviceSource, /export function normalizeDeviceTokenResponse/);
  assert.match(deviceSource, /access_token/);
  assert.match(deviceSource, /returned no \$\{missing\}/);
  assert.match(deviceSource, /Device login token request failed \(\$\{res\.status\}\)/);
  assert.match(mainSource, /let activeDeviceLoginAttempt = 0/);
  assert.match(mainSource, /let session: DeviceCodeSession;[\s\S]*session = await requestDeviceCode\(gateway\)/);
  assert.match(mainSource, /void pollDeviceToken\(session, gateway\)/);
  assert.match(mainSource, /const verificationLoginUri = deviceVerificationLoginUri\(session\.verificationUri\)/);
  assert.match(mainSource, /shell\.openExternal\(verificationLoginUri\)/);
  assert.match(mainSource, /attemptID !== activeDeviceLoginAttempt/);
  assert.match(mainSource, /auth:deviceCodeComplete'[\s\S]*success: false/);
});

test('default desktop sign-in uses device-code token exchange instead of web protocol redirect', () => {
  assert.match(mainSource, /console\.log\(app\.isPackaged \? '\[Auth\] Starting Device Code Flow\.\.\.'/);
  assert.match(mainSource, /await startDeviceCodeLoginFlow\(homeDir, gatewayInfo\.gateway, gatewayInfo, requestedOrgSlug\);[\s\S]*mode: 'device_code'/);
  assert.doesNotMatch(mainSource, /const loginUrl = getLoginUrl/);
  assert.doesNotMatch(mainSource, /mode: 'protocol'/);
});

test('desktop auth callback is OpenBrain-only', () => {
  const storeSource = read('authStore.ts');
  assert.match(storeSource, /openbrain:\/\/auth\/callback\?token=/);
  assert.match(storeSource, /parsedUrl\.protocol !== 'openbrain:'/);
  assert.match(storeSource, /parsedUrl\.hostname !== 'auth'/);
  assert.match(storeSource, /parsedUrl\.searchParams/);
  assert.doesNotMatch(storeSource, /opagent:\/\/auth\/callback/);
  assert.match(mainSource, /const PROTOCOL_NAMES = \['openbrain'\]/);
  assert.doesNotMatch(mainSource, /opagent:\/\/auth\/callback/);
  assert.match(mainSource, /function isAuthCallbackUrl\(url: string\)/);
});

test('auth profile reads are bound to the current signed-in account', () => {
  assert.match(mainSource, /function profileMatchesAuth\(config: AuthConfig, profile\?: UserProfile \| null\): profile is UserProfile/);
  assert.match(mainSource, /profile\.uid !== config\.uid/);
  assert.match(mainSource, /profileEmail[\s\S]*authEmail[\s\S]*profileEmail === authEmail/);
  assert.match(mainSource, /async function loadProfileForAuth\(homeDir: string, config: AuthConfig\)/);
  assert.match(mainSource, /await clearProfile\(homeDir\);/);
  assert.match(mainSource, /ipcMain\.handle\('auth:get'[\s\S]*loadProfileForAuth\(homeDir, config\)/);
  assert.match(mainSource, /ipcMain\.handle\('profile:get'[\s\S]*loadProfileForAuth\(homeDir, config\)/);
  assert.match(mainSource, /ipcMain\.handle\('auth:setActiveOrg'[\s\S]*loadProfileForAuth\(homeDir, next\)/);
});

test('desktop profile avatars use only canonical user avatar URLs', () => {
  assert.match(profileSource, /const CANONICAL_AVATAR_PATH_PREFIX = '\/v1\/user\/avatar\/';/);
  assert.match(profileSource, /export function resolveProfileAvatarUrl\(gateway: string, value: string \| undefined\)/);
  assert.match(profileSource, /filename\.includes\('\/'\)/);
  assert.match(profileSource, /filename\.includes\('\\\\'\)/);
  assert.match(profileSource, /filename\.includes\('\.\.'\)/);
  assert.match(profileSource, /parsed\.search \|\| parsed\.hash/);
  assert.match(profileSource, /avatar: resolveProfileAvatarUrl\(gateway, readMeAvatar\(data\)\)/);
  assert.doesNotMatch(profileSource, /localAvatar/);
  assert.doesNotMatch(mainSource, /cacheUserAvatar/);
  assert.doesNotMatch(mainSource, /avatar:cacheUser/);
});

test('OpenBrain Cloud auth-required responses do not clear global desktop auth', () => {
  assert.doesNotMatch(mainSource, /function invalidateAuthSessionForOpenBrainResponse/);
  assert.doesNotMatch(mainSource, /invalidateAuthSessionForOpenBrainResponse\(/);
  assert.match(mainSource, /await invalidateAuthSession\('session_expired'\)/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:listSources'[\s\S]*return await listOpenBrainSources\(context\)/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:query'[\s\S]*return await queryOpenBrainProvider\(\{ \.\.\.context, input \}\)/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:createSource'/);
  assert.match(mainSource, /context\.settings\?\.provider !== 'local'/);
  assert.match(mainSource, /code: 'openbrain_source_flow_required'/);
  assert.match(mainSource, /return await createOpenBrainSource\(\{[\s\S]*\.\.\.context,[\s\S]*name: input\?\.name,[\s\S]*localPath: input\?\.localPath,[\s\S]*\}\);/);
  assert.doesNotMatch(mainSource, /remoteHost = await resolveSshHostForConnect\(app\.getPath\('home'\), input\.remoteHost\);/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:removeSourceFromDevice'[\s\S]*return await removeOpenBrainSourceFromDevice\(\{/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:archiveSource'[\s\S]*return await archiveOpenBrainSource\(\{/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:sourceAction'[\s\S]*return await applyOpenBrainSourceAction\(\{/);
});
