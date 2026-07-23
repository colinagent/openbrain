import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const mainPath = path.join(__dirname, 'main.ts');
const source = fs.readFileSync(mainPath, 'utf8');

test('private auth services restrict managed model catalogs to the token-bound organization', () => {
  assert.match(source, /function isPrivateAuthService\(auth: AuthConfig\): boolean/);
  assert.match(source, /api\.op-agent\.com/);
  assert.match(source, /function isOfficialOpenBrainHost\(hostname: string\): boolean/);
  assert.match(source, /const boundOrg = orgs\.find\(\(org\) => org\.id === auth\.orgID\);/);
  assert.match(source, /const orgEntries: OpenBrainOrgEntry\[\] = \[\{\s*id: auth\.orgID,/s);
  assert.match(source, /if \(org\.id !== auth\.orgID\)/);
});

test('manual private gateway login uses the gateway as auth base when discovery data is absent', () => {
  assert.match(source, /const authBaseUrl = result\.baseUrl \|\| gatewayInfo\?\.baseUrl \|\| gateway;/);
  assert.match(source, /const authGateway = result\.gateway \|\| gatewayInfo\?\.gateway \|\| gateway;/);
});

test('private model refresh passes privateOnly into catalog merge', () => {
  assert.match(source, /privateOnly: isPrivateAuthService\(auth\),/);
  assert.match(source, /privateOnly: isPrivateAuthService\(next\),/);
});

test('workspace creation only uses the token-bound organization target', () => {
  assert.match(source, /workspaceCreationOrgTargets\(auth: AuthConfig, orgs: AuthOrgEntry\[\]\)/);
  assert.match(source, /return orgs\.filter\(\(org\) => org\.id === auth\.orgID\);/);
  assert.match(source, /ipcMain\.handle\('auth:listOrgs'/);
  assert.match(source, /ipcMain\.handle\('workspace:listTemplates', async \(_event, input\?: \{ orgID\?: string \| null; targetOrgID\?: string \| null \}/);
  assert.match(source, /ipcMain\.handle\('workspace:createFromTemplate', async \(_event, input\?: \{ templateID\?: string; storageBackend\?: string; provider\?: string; repositoryOwner\?: string; repositoryName\?: string; name\?: string; orgID\?: string \| null; targetOrgID\?: string \| null; localPath\?: string \}/);
  assert.match(source, /const orgID = normalizeActiveOrgID\(input\?\.targetOrgID\);/);
  assert.doesNotMatch(source, /normalizeActiveOrgID\(input\?\.targetOrgID \|\| input\?\.orgID\)/);
});
