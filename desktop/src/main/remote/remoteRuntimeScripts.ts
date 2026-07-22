import type { OpenBrainPlatform } from '../openbrain/releaseManifest';

export type RemoteRuntimeTarget = {
  platform: OpenBrainPlatform;
  home: string;
};

type InstallScriptOptions = {
  remotePort: number;
  version: string;
  bundleUrl: string;
  bundleSha256: string;
  bootstrapUrl: string;
  bootstrapSha256: string;
  target: RemoteRuntimeTarget;
};

function escapeSingleQuotes(input: string) {
  return input.replace(/'/g, `'\\''`);
}

function psQuote(input: string) {
  return `'${input.replace(/'/g, "''")}'`;
}

function posixQuote(input: string) {
  return `'${input.replace(/'/g, `'\\''`)}'`;
}

export function buildPowerShellCommand(script: string) {
  const encoded = Buffer.from(script, 'utf16le').toString('base64');
  return `powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand ${encoded}`;
}

function buildPosixCommand(script: string) {
  return `sh -lc '${escapeSingleQuotes(script)}'`;
}

function isWindowsPath(pathValue: string) {
  return /^[A-Za-z]:[\\/]/.test(pathValue) || pathValue.includes('\\');
}

function joinRemotePath(root: string, ...parts: string[]) {
  const windows = isWindowsPath(root);
  const sep = windows ? '\\' : '/';
  const cleanedRoot = root.replace(/[\\/]+$/, '');
  const cleanedParts = parts.map((part) => part.replace(/^[\\/]+|[\\/]+$/g, '')).filter(Boolean);
  return [cleanedRoot, ...cleanedParts].join(sep);
}

function remoteBaseDir(target: RemoteRuntimeTarget) {
  return joinRemotePath(target.home, '.openbrain');
}

function remoteBootstrapPath(target: RemoteRuntimeTarget) {
  const name = target.platform.os === 'windows' ? 'openbrain-bootstrap.exe' : 'openbrain-bootstrap';
  return joinRemotePath(remoteBaseDir(target), 'bin', name);
}

export function buildStartExistingScript(options: { remotePort: number; target: RemoteRuntimeTarget }) {
  const baseDir = remoteBaseDir(options.target);
  const bootstrap = remoteBootstrapPath(options.target);
  if (options.target.platform.os === 'windows') {
    return buildPowerShellCommand([
      '$ErrorActionPreference = "Stop"',
      `$bootstrap = ${psQuote(bootstrap)}`,
      `if (!(Test-Path -LiteralPath $bootstrap)) { throw "remote runtime bootstrapper missing: $bootstrap" }`,
      `& $bootstrap start --base-dir ${psQuote(baseDir)} --port ${options.remotePort} --json-events`,
      'exit $LASTEXITCODE',
    ].join('\n'));
  }

  const script = [
    'set -e',
    `BOOTSTRAP=${posixQuote(bootstrap)}`,
    `BASE_DIR=${posixQuote(baseDir)}`,
    'if [ ! -x "$BOOTSTRAP" ]; then echo "remote runtime bootstrapper missing: $BOOTSTRAP" >&2; exit 1; fi',
    `"$BOOTSTRAP" start --base-dir "$BASE_DIR" --port ${options.remotePort} --json-events`,
  ].join('\n');
  return buildPosixCommand(script);
}

export function buildInstallScript(options: InstallScriptOptions) {
  const baseDir = remoteBaseDir(options.target);
  const bootstrap = remoteBootstrapPath(options.target);
  if (options.target.platform.os === 'windows') {
    return buildPowerShellCommand([
      '$ErrorActionPreference = "Stop"',
      '$ProgressPreference = "SilentlyContinue"',
      `$baseDir = ${psQuote(baseDir)}`,
      `$bootstrap = ${psQuote(bootstrap)}`,
      '$binDir = Split-Path -Parent $bootstrap',
      'New-Item -ItemType Directory -Force -Path $binDir | Out-Null',
      '$tmp = "$bootstrap.download"',
      `Invoke-WebRequest -Uri ${psQuote(options.bootstrapUrl)} -OutFile $tmp`,
      '$actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $tmp).Hash.ToLowerInvariant()',
      `$expected = ${psQuote(options.bootstrapSha256.toLowerCase())}`,
      'if ($actual -ne $expected) { throw "bootstrap sha256 mismatch: expected $expected got $actual" }',
      'Move-Item -Force -LiteralPath $tmp -Destination $bootstrap',
      `& $bootstrap ensure --base-dir $baseDir --port ${options.remotePort} --version ${psQuote(options.version)} --bundle-url ${psQuote(options.bundleUrl)} --bundle-sha256 ${psQuote(options.bundleSha256)} --json-events`,
      'exit $LASTEXITCODE',
    ].join('\n'));
  }

  const script = [
    'set -e',
    `BASE_DIR=${posixQuote(baseDir)}`,
    `BOOTSTRAP=${posixQuote(bootstrap)}`,
    `BOOTSTRAP_URL=${posixQuote(options.bootstrapUrl)}`,
    `BOOTSTRAP_SHA256=${posixQuote(options.bootstrapSha256)}`,
    `VERSION=${posixQuote(options.version)}`,
    `BUNDLE_URL=${posixQuote(options.bundleUrl)}`,
    `BUNDLE_SHA256=${posixQuote(options.bundleSha256)}`,
    'command -v curl >/dev/null 2>&1 || { echo "curl not found" >&2; exit 1; }',
    'if command -v sha256sum >/dev/null 2>&1; then SHA256_CMD="sha256sum"; elif command -v shasum >/dev/null 2>&1; then SHA256_CMD="shasum -a 256"; else echo "sha256sum/shasum not found" >&2; exit 1; fi',
    'mkdir -p "$(dirname "$BOOTSTRAP")"',
    'TMP_BOOTSTRAP="${BOOTSTRAP}.download"',
    'curl -fL "$BOOTSTRAP_URL" -o "$TMP_BOOTSTRAP"',
    'ACTUAL="$($SHA256_CMD "$TMP_BOOTSTRAP" | awk \'{print $1}\')"',
    'if [ "$ACTUAL" != "$BOOTSTRAP_SHA256" ]; then echo "bootstrap sha256 mismatch: expected $BOOTSTRAP_SHA256 got $ACTUAL" >&2; exit 1; fi',
    'mv -f "$TMP_BOOTSTRAP" "$BOOTSTRAP"',
    'chmod +x "$BOOTSTRAP"',
    `"$BOOTSTRAP" ensure --base-dir "$BASE_DIR" --port ${options.remotePort} --version "$VERSION" --bundle-url "$BUNDLE_URL" --bundle-sha256 "$BUNDLE_SHA256" --json-events`,
  ].join('\n');
  return buildPosixCommand(script);
}
