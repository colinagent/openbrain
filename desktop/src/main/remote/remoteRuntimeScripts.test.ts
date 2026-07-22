import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildInstallScript,
  buildStartExistingScript,
  type RemoteRuntimeTarget,
} from './remoteRuntimeScripts';

function decodePowerShellCommand(command: string) {
  const prefix = 'powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand ';
  assert.ok(command.startsWith(prefix), `command should use powershell.exe EncodedCommand: ${command}`);
  return Buffer.from(command.slice(prefix.length), 'base64').toString('utf16le');
}

const windowsTarget: RemoteRuntimeTarget = {
  platform: { os: 'windows', arch: 'amd64' },
  home: 'C:\\Users\\Open Brain',
};

const linuxTarget: RemoteRuntimeTarget = {
  platform: { os: 'linux', arch: 'amd64' },
  home: '/home/openbrain',
};

test('buildInstallScript generates Windows PowerShell bootstrap install', () => {
  const script = decodePowerShellCommand(buildInstallScript({
    remotePort: 19530,
    version: '0.8.0',
    bundleUrl: 'https://download.example/runtime/windows-amd64.tar.gz',
    bundleSha256: 'bundle-sha',
    bootstrapUrl: 'https://download.example/runtime/openbrain-bootstrap-windows-amd64.exe',
    bootstrapSha256: 'ABCDEF',
    target: windowsTarget,
  }));

  assert.match(script, /\$baseDir = 'C:\\Users\\Open Brain\\.openbrain'/);
  assert.match(script, /\$bootstrap = 'C:\\Users\\Open Brain\\.openbrain\\bin\\openbrain-bootstrap\.exe'/);
  assert.match(script, /Invoke-WebRequest -Uri 'https:\/\/download\.example\/runtime\/openbrain-bootstrap-windows-amd64\.exe' -OutFile \$tmp/);
  assert.match(script, /Get-FileHash -Algorithm SHA256 -LiteralPath \$tmp/);
  assert.match(script, /\$expected = 'abcdef'/);
  assert.match(script, /& \$bootstrap ensure --base-dir \$baseDir --port 19530 --version '0\.8\.0' --bundle-url 'https:\/\/download\.example\/runtime\/windows-amd64\.tar\.gz' --bundle-sha256 'bundle-sha' --json-events/);
});

test('buildStartExistingScript generates Windows PowerShell bootstrap start', () => {
  const script = decodePowerShellCommand(buildStartExistingScript({
    remotePort: 19531,
    target: windowsTarget,
  }));

  assert.match(script, /Test-Path -LiteralPath \$bootstrap/);
  assert.match(script, /& \$bootstrap start --base-dir 'C:\\Users\\Open Brain\\.openbrain' --port 19531 --json-events/);
});

test('buildInstallScript uses POSIX sh without bash', () => {
  const command = buildInstallScript({
    remotePort: 19530,
    version: '0.8.0',
    bundleUrl: 'https://download.example/runtime/linux-amd64.tar.gz',
    bundleSha256: 'bundle-sha',
    bootstrapUrl: 'https://download.example/runtime/openbrain-bootstrap-linux-amd64',
    bootstrapSha256: 'bootstrap-sha',
    target: linuxTarget,
  });

  assert.match(command, /^sh -lc '/);
  assert.doesNotMatch(command, /\bbash\b/);
  assert.match(command, /BOOTSTRAP=.*\/home\/openbrain\/\.openbrain\/bin\/openbrain-bootstrap/);
  assert.match(command, /"\$BOOTSTRAP" ensure --base-dir "\$BASE_DIR" --port 19530/);
});
