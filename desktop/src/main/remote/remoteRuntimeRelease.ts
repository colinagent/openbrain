import {
  fetchReleaseManifest,
  parseRemotePlatform,
  pickPlatformAssets,
  type OpenBrainPlatform,
} from '../openbrain/releaseManifest';
import type { SshHostWithSecrets } from '../ssh/sshTypes';
import { runSsh } from './ssh2Transport';
import { buildPowerShellCommand, type RemoteRuntimeTarget } from './remoteRuntimeScripts';

const MANIFEST_FETCH_TIMEOUT_MS = 8_000;

export type LatestRelease = {
  version: string;
  bundleUrl: string;
  bundleSha256: string;
  bootstrapUrl: string;
  bootstrapSha256: string;
};

export async function resolveRemoteRuntimeTarget(host: SshHostWithSecrets): Promise<RemoteRuntimeTarget> {
  try {
    const { stdout } = await runSsh(
      host,
      `sh -lc 'printf "os=%s\\narch=%s\\nhome=%s\\n" "$(uname -s)" "$(uname -m)" "$HOME"'`,
      10_000,
    );
    const values = parseProbeOutput(stdout);
    return {
      platform: parseRemotePlatform(values.os ?? '', values.arch ?? ''),
      home: values.home || '/',
    };
  } catch (posixError) {
    try {
      const { stdout } = await runSsh(
        host,
        buildPowerShellCommand([
          '$ErrorActionPreference = "Stop"',
          '$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }',
          'Write-Output "os=windows"',
          'Write-Output "arch=$arch"',
          'Write-Output "home=$env:USERPROFILE"',
        ].join('\n')),
        10_000,
      );
      const values = parseProbeOutput(stdout);
      return {
        platform: parseRemotePlatform(values.os ?? 'windows', values.arch ?? ''),
        home: values.home || '/',
      };
    } catch (windowsError) {
      throw new Error(`Remote platform probe failed. POSIX probe: ${formatProbeError(posixError)}. Windows probe: ${formatProbeError(windowsError)}`);
    }
  }
}

function parseProbeOutput(stdout: string): Record<string, string> {
  const values: Record<string, string> = {};
  for (const line of String(stdout ?? '').split(/\r?\n/)) {
    const idx = line.indexOf('=');
    if (idx <= 0) {
      continue;
    }
    values[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
  }
  return values;
}

function formatProbeError(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

export async function fetchLatestRuntimeRelease(platform: OpenBrainPlatform, manifestUrl: string): Promise<LatestRelease> {
  const manifest = await fetchReleaseManifest(manifestUrl, { timeoutMs: MANIFEST_FETCH_TIMEOUT_MS });
  const picked = pickPlatformAssets(manifest, platform);
  if (!picked.assets.bootstrap) {
    throw new Error('manifest.json missing bootstrap asset for remote runtime install');
  }
  return {
    version: picked.version,
    bundleUrl: picked.assets.bundle.url,
    bundleSha256: picked.assets.bundle.sha256,
    bootstrapUrl: picked.assets.bootstrap.url,
    bootstrapSha256: picked.assets.bootstrap.sha256,
  };
}
