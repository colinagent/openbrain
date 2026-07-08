import { parse as parseJsonc } from 'jsonc-parser';

import type { SshHostWithSecrets } from '../ssh/sshTypes';
import { buildPowerShellCommand, type RemoteRuntimeTarget } from './remoteRuntimeScripts';
import { runSsh } from './ssh2Transport';

function isWindowsTarget(target: RemoteRuntimeTarget) {
  return target.platform.os === 'windows';
}

function psQuote(input: string) {
  return `'${input.replace(/'/g, "''")}'`;
}

function posixQuote(input: string) {
  return `'${input.replace(/'/g, `'\\''`)}'`;
}

function userSettingsPathCandidates(target: RemoteRuntimeTarget) {
  const home = target.home.replace(isWindowsTarget(target) ? /[\\/]+$/ : /\/+$/, '');
  if (isWindowsTarget(target)) {
    return [
      `${home}\\.openbrain\\configs\\settings\\user.jsonc`,
      `${home}\\.openbrain\\configs\\settings\\user.json`,
      `${home}\\.openbrain\\settings\\user.jsonc`,
      `${home}\\.openbrain\\settings\\user.json`,
    ];
  }
  return [
    `${home}/.openbrain/configs/settings/user.jsonc`,
    `${home}/.openbrain/configs/settings/user.json`,
    `${home}/.openbrain/settings/user.jsonc`,
    `${home}/.openbrain/settings/user.json`,
  ];
}

export async function getRemoteDefaultWorkspace(host: SshHostWithSecrets, target: RemoteRuntimeTarget): Promise<string | null> {
  // Remote machine controls its own defaultWorkspace via ~/.openbrain/configs/settings/user.jsonc.
  // Legacy remotes may still have ~/.openbrain/settings/user.jsonc or .json.
  try {
    const settingsPaths = userSettingsPathCandidates(target);
    const command = isWindowsTarget(target)
      ? buildPowerShellCommand([
          '$ErrorActionPreference = "SilentlyContinue"',
          ...settingsPaths.map(
            (settingsPath) => `$path = ${psQuote(settingsPath)}; if (Test-Path -LiteralPath $path) { Get-Content -Raw -LiteralPath $path; exit 0 }`
          ),
        ].join('\n'))
      : `sh -lc '${settingsPaths.map((settingsPath) => `cat ${posixQuote(settingsPath)} 2>/dev/null`).join(' || ')} || true'`;
    const { stdout } = await runSsh(host, command, 10_000);
    const raw = String(stdout ?? '').trim();
    if (!raw) {
      return null;
    }
    const parsed = parseJsonc(raw) as { defaultWorkspace?: unknown } | undefined;
    const value = typeof parsed?.defaultWorkspace === 'string' ? parsed.defaultWorkspace.trim() : '';
    return value || null;
  } catch {
    return null;
  }
}
