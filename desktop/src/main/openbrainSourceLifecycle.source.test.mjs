import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function read(relativePath) {
  return readFileSync(path.join(__dirname, relativePath), 'utf8');
}

test('OpenBrain cloud source matching prefers GitHub external ID', () => {
  const serverSource = read('../../../server/internal/server/gbrain/cloud.go');

  assert.match(serverSource, /func cloudWorkspaceRepositoryMatches/);
  assert.match(serverSource, /workspaceExternalID := normalizeRepoExternalID\(workspace\.RepoExternalID\)/);
  assert.match(serverSource, /repoExternalID := normalizeRepoExternalID\(repo\.ExternalID\)/);
  assert.match(serverSource, /return workspaceExternalID == repoExternalID/);
  assert.match(serverSource, /workspaceOwnerName := repoOwnerNameKey\(workspace\.RepoProvider, workspace\.RepoOwner, workspace\.RepoName\)/);
  assert.match(serverSource, /repoKeysIntersect\(cloudWorkspaceRepoKeys\(workspace\), githubRepoRefKeys\(repo\)\)/);
});

test('OpenBrain workspace index rejects path collision across workspace IDs', () => {
  const workspaceSource = read('./workspace/openbrainWorkspace.ts');

  assert.match(workspaceSource, /workspace_path_conflict/);
  assert.match(workspaceSource, /item\.workspaceID !== entry\.workspaceID[\s\S]*normalizeWorkspacePathForIndex\(item\.path\) === normalizedPath/);
  assert.match(workspaceSource, /filter\(\(item\) => item\.workspaceID !== entry\.workspaceID\)/);
  assert.doesNotMatch(workspaceSource, /item\.workspaceID !== entry\.workspaceID && item\.path !== entry\.path/);
});
