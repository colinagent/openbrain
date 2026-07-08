import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(
  path.join(__dirname, 'localReleaseInstall.ts'),
  'utf8',
);

test('local release install restores built-in agents and gbrain assets', () => {
  assert.match(
    source,
    /const agentsRoot = path\.join\(opRoot, ["']agents["']\);/,
  );
  assert.match(
    source,
    /const toolsRoot = path\.join\(opRoot, ["']tools["']\);/,
  );
  assert.match(
    source,
    /const skillsRoot = path\.join\(opRoot, ["']skills["']\);/,
  );
  assert.match(
    source,
    /const coderManifestTarget = path\.join\(\s*agentsRoot,\s*["']coder["'],\s*["']\.agent["'],\s*["']AGENT\.md["'],\s*\);/s,
  );
  assert.match(
    source,
    /const simpleMemoryManifestTarget = path\.join\(\s*agentsRoot,\s*["']simple-memory["'],\s*["']\.agent["'],\s*["']AGENT\.md["'],\s*\);/s,
  );
  assert.match(
    source,
    /const gbrainManifestTarget = path\.join\(\s*agentsRoot,\s*["']gbrain["'],\s*["']\.agent["'],\s*["']AGENT\.md["'],\s*\);/s,
  );
  assert.match(
    source,
    /const gbrainTarget = path\.join\(runtimeBinDir, `gbrain\$\{exe\}`\);/,
  );
  assert.match(
    source,
    /const coderTarget = path\.join\(\s*agentsRoot,\s*["']coder["'],\s*["']\.agent["'],\s*["']bin["'],\s*`coder\$\{exe\}`,\s*\);/s,
  );
  assert.match(
    source,
    /const serverBinDir = path\.join\(serverAgentDir, ["']bin["']\);/,
  );
  assert.match(
    source,
    /const serverTarget = path\.join\(serverBinDir, `openbrain-server\$\{exe\}`\);/,
  );
  assert.match(source, /coderTarget,/);
  assert.match(source, /simpleMemoryManifestTarget,/);
  assert.match(source, /gbrainManifestTarget,/);
  assert.match(source, /gbrainCloudToolManifestTarget,/);
  assert.match(source, /openBrainCloudSyncSkillManifestTarget,/);
  assert.match(source, /openBrainCloudSyncHelperTarget,/);
  assert.match(source, /parseYaml/);
  assert.match(source, /async function projectSystemToolBins\(opRoot: string, homeDir: string, os: OpenBrainOs\)/);
  assert.match(source, /await ensureUserPathContains\(runtimeBinDir, homeDir, os\);/);
  assert.match(
    source,
    /copyDirForce\(\s*path\.join\(extractedDir, ["']agents["'], ["']gbrain["']\),\s*path\.join\(agentsRoot, ["']gbrain["']\),\s*\)/s,
  );
  assert.match(
    source,
    /copyDirForce\(\s*path\.join\(extractedDir, ["']agents["'], ["']coder["']\),\s*path\.join\(agentsRoot, ["']coder["']\),\s*\)/s,
  );
  assert.match(
    source,
    /copyDirForce\(\s*path\.join\(extractedDir, ["']agents["'], ["']simple-memory["']\),\s*path\.join\(agentsRoot, ["']simple-memory["']\),\s*\)/s,
  );
  assert.match(
    source,
    /copyDirForce\(\s*path\.join\(extractedDir, ["']agents["'], ["']opagent-server["'], ["']\.agent["'], ["']bin["']\),\s*serverBinDir,\s*\)/s,
  );
  assert.match(
    source,
    /copyDirForce\(\s*path\.join\(extractedDir, ["']tools["']\),\s*toolsRoot,?\s*\)/s,
  );
  assert.match(
    source,
    /copyDirForce\(\s*path\.join\(extractedDir, ["']skills["'], ["']openbrain-cloud-sync["']\),\s*path\.join\(skillsRoot, ["']openbrain-cloud-sync["']\),\s*\)/s,
  );
  assert.match(source, /fs\.chmod\(gbrainTarget, 0o755\)/);
  assert.match(source, /fs\.chmod\(openBrainCloudSyncHelperTarget, 0o755\)/);
});
