export type OpenBrainOs = 'darwin' | 'linux' | 'windows';
export type OpenBrainArch = 'amd64' | 'arm64';

export type OpenBrainPlatform = {
  os: OpenBrainOs;
  arch: OpenBrainArch;
};

export type ReleaseAsset = {
  url: string;
  sha256: string;
};

export type ReleasePlatformAssets = {
  bundle: ReleaseAsset;
  bootstrap?: ReleaseAsset;
};

export type RuntimeReleaseManifest = {
  version: string;
  generatedAt?: string;
  assets: Record<string, ReleasePlatformAssets>;
};

export const DEFAULT_RUNTIME_MANIFEST_URL =
  'https://download.op-agent.com/runtime/latest/manifest.json';

export function getLocalPlatform(): OpenBrainPlatform {
  const os: OpenBrainOs = (() => {
    switch (process.platform) {
      case 'darwin':
        return 'darwin';
      case 'linux':
        return 'linux';
      case 'win32':
        return 'windows';
      default:
        throw new Error(`Unsupported platform: ${process.platform}`);
    }
  })();

  const arch: OpenBrainArch = (() => {
    switch (process.arch) {
      case 'x64':
        return 'amd64';
      case 'arm64':
        return 'arm64';
      default:
        throw new Error(`Unsupported arch: ${process.arch}`);
    }
  })();

  return { os, arch };
}

export function getPlatformKey(platform: OpenBrainPlatform): string {
  return `${platform.os}-${platform.arch}`;
}

export function parseRemotePlatform(unameS: string, unameM: string): OpenBrainPlatform {
  const osRaw = String(unameS || '').trim();
  const archRaw = String(unameM || '').trim();

  const os: OpenBrainOs = (() => {
    if (/^darwin$/i.test(osRaw)) return 'darwin';
    if (/^linux$/i.test(osRaw)) return 'linux';
    if (/^windows$/i.test(osRaw) || /^win32$/i.test(osRaw)) return 'windows';
    throw new Error(`Unsupported remote OS from uname -s: ${osRaw || '<empty>'}`);
  })();

  const arch: OpenBrainArch = (() => {
    if (archRaw === 'x86_64' || archRaw === 'amd64') return 'amd64';
    if (archRaw === 'aarch64' || archRaw === 'arm64') return 'arm64';
    throw new Error(`Unsupported remote arch from uname -m: ${archRaw || '<empty>'}`);
  })();

  return { os, arch };
}

export async function fetchReleaseManifest(
  baseUrl = DEFAULT_RUNTIME_MANIFEST_URL,
  options?: { timeoutMs?: number },
): Promise<RuntimeReleaseManifest> {
  const timeoutMs = options?.timeoutMs ?? 5_000;
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  let res: Response;
  try {
    res = await fetch(baseUrl, {
      headers: {
        Accept: 'application/json',
      },
      signal: controller.signal,
    });
  } finally {
    clearTimeout(timeout);
  }
  if (!res.ok) {
    throw new Error(`Failed to fetch manifest: ${res.status} ${res.statusText}`);
  }
  const data: any = await res.json();
  const version = String(data?.version ?? '').trim();
  if (!version) {
    throw new Error('manifest.json missing version');
  }
  const assets = data?.assets;
  if (!assets || typeof assets !== 'object') {
    throw new Error('manifest.json missing assets');
  }
  return data as RuntimeReleaseManifest;
}

export function pickPlatformAssets(
  manifest: RuntimeReleaseManifest,
  platform: OpenBrainPlatform,
): { version: string; assets: ReleasePlatformAssets } {
  const key = getPlatformKey(platform);
  const entry: any = (manifest as any)?.assets?.[key];
  if (!entry) {
    throw new Error(`manifest.json missing assets for platform ${key}`);
  }

  const bundleUrl = String(entry?.bundle?.url ?? '').trim();
  const bundleSha = String(entry?.bundle?.sha256 ?? '').trim();
  const bootstrapUrl = String(entry?.bootstrap?.url ?? '').trim();
  const bootstrapSha = String(entry?.bootstrap?.sha256 ?? '').trim();

  if (!bundleUrl || !bundleSha) {
    throw new Error(`manifest.json invalid assets payload for platform ${key}`);
  }

  const assets: ReleasePlatformAssets = {
    bundle: { url: bundleUrl, sha256: bundleSha },
  };
  if (bootstrapUrl || bootstrapSha) {
    if (!bootstrapUrl || !bootstrapSha) {
      throw new Error(`manifest.json invalid bootstrap asset payload for platform ${key}`);
    }
    assets.bootstrap = { url: bootstrapUrl, sha256: bootstrapSha };
  }

  return {
    version: String(manifest.version),
    assets,
  };
}
