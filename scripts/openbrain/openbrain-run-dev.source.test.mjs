import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(
  path.join(__dirname, 'openbrain-run-dev.sh'),
  'utf8',
);

test('dev-all installs built-in tools, gbrain agent, and packaged gbrain binary', () => {
  assert.match(
    source,
    /OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT="\$\{OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT:-\$\{REPO_ROOT\}\/agents\/gbrain\}"/,
  );
  assert.match(
    source,
    /OPENBRAIN_GBRAIN_SOURCE_ROOT="\$\{OPENBRAIN_GBRAIN_SOURCE_ROOT:-\$\{REPO_ROOT\}\/\.\.\/gbrain\}"/,
  );
  assert.match(
    source,
    /OPENBRAIN_TOOLS_SOURCE_ROOT="\$\{OPENBRAIN_TOOLS_SOURCE_ROOT:-\$\{REPO_ROOT\}\/tools\}"/,
  );
  assert.match(
    source,
    /OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT="\$\{OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT:-\$\{REPO_ROOT\}\/skills\/openbrain-cloud-sync\}"/,
  );
  assert.match(source, /GBRAIN_BIN="\$\{RUNTIME_BIN_DIR\}\/gbrain"/);
  assert.match(
    source,
    /SERVER_BIN="\$\{SERVER_AGENT_DIR\}\/\.agent\/bin\/openbrain-server"/,
  );
  assert.match(
    source,
    /CODER_AGENT_BIN="\$\{CODER_AGENT_DIR\}\/\.agent\/bin\/coder"/,
  );
  assert.match(source, /LEGACY_OPAGENT_AGENT_DIR="\$\{OP_HOME\}\/agents\/opagent"/);
  assert.match(
    source,
    /OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT="\$\{OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT:-\$\{REPO_ROOT\}\/agents\/simple-memory\}"/,
  );
  assert.match(source, /CLOUD_SYNC_SKILL_BIN="\$\{CLOUD_SYNC_SKILL_DIR\}\/bin\/openbrain-cloud-sync-helper"/);
  assert.match(
    source,
    /function build_and_install_gbrain\(\)|build_and_install_gbrain\(\) \{/,
  );
  assert.match(
    source,
    /bun build --compile --outfile "\$tmp_bin" src\/cli\.ts/,
  );
  assert.match(
    source,
    /function install_builtin_tools\(\)|install_builtin_tools\(\) \{/,
  );
  assert.match(
    source,
    /cp -R "\$tool_src" "\$tool_dest"/,
  );
  assert.match(source, /project_system_tool_bins/);
  assert.match(source, /manifest_has_system_tag/);
  assert.match(
    source,
    /function sync_gbrain_agent\(\)|sync_gbrain_agent\(\) \{/,
  );
  assert.match(source, /rm -rf "\$\{OP_HOME\}\/agents\/brain"/);
  assert.match(
    source,
    /cp -R "\$OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT" "\$GBRAIN_AGENT_DIR"/,
  );
  assert.match(
    source,
    /install_simple_memory_agent\(\) \{/,
  );
  assert.match(
    source,
    /build_and_install_gbrain\s+install_builtin_tools\s+build_and_install_coder_agent\s+remove_legacy_opagent_agent\s+install_simple_memory_agent\s+build_and_install_cloud_sync_skill\s+sync_gbrain_agent/m,
  );
  assert.match(source, /remove_legacy_opagent_agent\(\) \{/);
  assert.doesNotMatch(source, /build_and_install_sync_agent/);
});
