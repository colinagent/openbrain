import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const repoRoot = process.cwd().endsWith('desktop')
  ? process.cwd()
  : path.join(process.cwd(), 'desktop');

const resolverSource = readFileSync(path.join(repoRoot, 'src/main/auth/openbrainApiBase.ts'), 'utf8');
const workspaceSource = readFileSync(path.join(repoRoot, 'src/main/workspace/openbrainWorkspace.ts'), 'utf8');
const brainProviderSource = readFileSync(path.join(repoRoot, 'src/main/openbrain/brainProvider.ts'), 'utf8');
const localGBrainQuerySource = readFileSync(path.join(repoRoot, 'src/main/openbrain/localGBrainQuery.ts'), 'utf8');
const settingsSource = readFileSync(path.join(repoRoot, 'src/main/settings/settingsStore.ts'), 'utf8');
const mainSource = readFileSync(path.join(repoRoot, 'src/main/main.ts'), 'utf8');
const preloadSource = readFileSync(path.join(repoRoot, 'src/main/preload.ts'), 'utf8');
const openBrainServiceSource = readFileSync(path.join(repoRoot, 'src/renderer/services/openBrainService.ts'), 'utf8');
const serverMainSource = readFileSync(path.join(repoRoot, '../server/cmd/openbrain-server/main.go'), 'utf8');
const serverGBrainSource = readFileSync(path.join(repoRoot, '../server/internal/server/gbrain/service.go'), 'utf8');

test('OpenBrain API base uses the signed-in gateway directly', () => {
  assert.doesNotMatch(resolverSource, /OFFICIAL_OPENBRAIN_API_BASE/);
  assert.doesNotMatch(resolverSource, /www\.openbrain\.io/);
  assert.match(resolverSource, /url\.pathname\.replace/);
  assert.doesNotMatch(resolverSource, /'\/gateway'/);
});

test('OpenBrain workspace callers use the shared API base resolver', () => {
  assert.match(workspaceSource, /resolveOpenBrainAPIBase/);
  assert.doesNotMatch(workspaceSource, /process\.env\.OPENBRAIN_API_URL \|\| ''/);
});

test('OpenBrain workspace APIs enforce the token-bound organization', () => {
  assert.match(workspaceSource, /function requireBoundOrgID\(/);
  assert.match(workspaceSource, /tenant_context_mismatch: workspace organization does not match the authenticated organization/);
  assert.match(workspaceSource, /const normalizedOrgID = requireBoundOrgID\(auth, orgID\);/);
  assert.match(workspaceSource, /requireBoundOrgID\(auth, result\.orgID, true\);/);
  assert.doesNotMatch(workspaceSource, /['"]X-Org-ID['"]/);
});

test('OpenBrain defaults to the authenticated cloud provider', () => {
  assert.match(settingsSource, /openBrain:\s*\{[\s\S]*provider:\s*'cloud'/);
  assert.match(settingsSource, /normalizeOpenBrainUserSettings/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:getProvider'/);
  assert.match(mainSource, /ipcMain\.handle\('openBrain:setProvider'/);
  assert.match(preloadSource, /openBrain:\s*\{[\s\S]*getProvider:[\s\S]*setProvider:[\s\S]*listSources:[\s\S]*query:[\s\S]*createSource:/);
});

test('OpenBrain renderer routes cloud source operations through the active workspace runtime server', () => {
  assert.match(openBrainServiceSource, /resolveOpenBrainBaseUrl/);
  assert.match(openBrainServiceSource, /const targetTabId = workspaceTabId \|\| getActiveWorkspaceTabId\(\);/);
  assert.match(openBrainServiceSource, /useAppStore\.getStoreByTabId\(targetTabId\)\.getState\(\)/);
  assert.match(openBrainServiceSource, /remoteSession\?\.localPort/);
  assert.match(openBrainServiceSource, /\/v1\/openbrain\/cloud\/sources/);
  assert.match(openBrainServiceSource, /\/v1\/openbrain\/cloud\/sources\/action/);
  assert.match(openBrainServiceSource, /\/v1\/openbrain\/cloud\/query/);
  assert.match(openBrainServiceSource, /window\.electronAPI\?\.openBrain\?\.getProvider/);
});

test('OpenBrain cloud provider calls authenticated Brain APIs', () => {
  assert.match(brainProviderSource, /\/v1\/me\/brain\/workspaces/);
  assert.match(brainProviderSource, /\/v1\/me\/brain\/search/);
  assert.match(brainProviderSource, /\/v1\/orgs\/\$\{encodeURIComponent\(workspaceOrgID/);
  assert.match(brainProviderSource, /Authorization:\s*`Bearer \$\{auth\.token\}`/);
  assert.match(brainProviderSource, /auth_required/);
  assert.match(brainProviderSource, /isAuthInvalidResponse\(res\.status, body\.error\)/);
  assert.match(brainProviderSource, /isOpenBrainCloudAuthRequiredFailure/);
  assert.match(brainProviderSource, /authRequired:\s*true/);
  assert.match(brainProviderSource, /cloud_unauthorized/);
  assert.match(brainProviderSource, /authRequired:\s*false/);
  assert.doesNotMatch(brainProviderSource, /error:\s*['"]unauthorized['"]/);
  assert.match(brainProviderSource, /listWorkspaceTemplates\(auth\)/);
  assert.match(brainProviderSource, /templates\.templates\.find\(\(template\) => template\.templateID === CLOUD_WORKSPACE_TEMPLATE_ID\)/);
  assert.match(brainProviderSource, /firstUsableGitHubOwnerFromProviders/);
  assert.match(brainProviderSource, /const githubConnected = Boolean\(owner\)/);
});

test('OpenBrain local provider keeps configurable GBrain modes', () => {
  assert.match(workspaceSource, /local\?\.cliPath \|\| process\.env\.GBRAIN_CLI_PATH/);
  assert.match(workspaceSource, /env\.GBRAIN_DATABASE_URL = databaseUrl/);
  assert.match(workspaceSource, /delete env\.DATABASE_URL/);
  assert.match(workspaceSource, /env\.GBRAIN_HOME = configHome/);
  assert.match(workspaceSource, /database_path/);
  assert.match(workspaceSource, /remote_mcp/);
  assert.match(workspaceSource, /remoteMcpClientID/);
  assert.match(workspaceSource, /oauth_client_id:\s*clientID/);
  assert.doesNotMatch(workspaceSource, /oauth_client_secret/);
  assert.match(workspaceSource, /GBRAIN_REMOTE_CLIENT_SECRET/);
  assert.match(workspaceSource, /sourceID:\s*'remote-mcp'/);
  assert.match(localGBrainQuerySource, /remote_mcp_query_unsupported/);
  assert.match(brainProviderSource, /remote_mcp_create_unsupported/);
  assert.match(workspaceSource, /brain\.pglite/);
  assert.match(brainProviderSource, /listConfiguredGBrainSourceWorkspaces/);
  assert.match(brainProviderSource, /queryConfiguredGBrain/);
});

test('OpenBrain uses the packaged GBrain binary before PATH fallback', () => {
  assert.match(workspaceSource, /GBRAIN_CLI_PATH/);
  assert.match(workspaceSource, /path\.join\(homeDir,\s*'\.openbrain',\s*'bin'\)/);
  assert.match(workspaceSource, /path\.join\(bundledGBrainBinDir\(homeDir\),\s*gbrainExecutableName\(\)\)/);
  assert.match(workspaceSource, /if \(await pathExists\(bundled\)\)/);
  assert.match(workspaceSource, /return gbrainExecutableName\(\);/);
  assert.match(serverGBrainSource, /GBRAIN_CLI_PATH/);
  assert.match(serverGBrainSource, /filepath\.Join\(s\.baseDir, "bin", gbrainExecutableName\(\)\)/);
  assert.match(serverGBrainSource, /return gbrainExecutableName\(\)/);
});

test('legacy local server GBrain APIs remain available for compatibility', () => {
  assert.match(serverMainSource, /router\.GET\("\/v1\/openbrain\/sources", gbrainHandler\.ListSources\)/);
  assert.match(serverMainSource, /router\.POST\("\/v1\/openbrain\/query", gbrainHandler\.Query\)/);
  assert.match(serverGBrainSource, /\[\]string\{"sources", "list", "--json"\}/);
  assert.match(serverGBrainSource, /\[\]string\{"call", "query", string\(rawPayload\)\}/);
});

test('OpenBrain source does not restore removed streaming Brain IPC endpoints', () => {
  const combined = [workspaceSource, mainSource, preloadSource].join('\n');
  assert.doesNotMatch(combined, /\/v1\/me\/brain\/chat/);
  assert.doesNotMatch(combined, /\/v1\/openbrain-cloud\/brain\/chat/);
  assert.doesNotMatch(combined, /listUserBrainWorkspaces/);
  assert.doesNotMatch(combined, /workspace:streamBrainChat/);
  assert.doesNotMatch(combined, /workspace:streamEnterpriseBrainChat/);
});
