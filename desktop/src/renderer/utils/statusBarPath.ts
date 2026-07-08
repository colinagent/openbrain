import { isPathInsideRoot } from './chatAgentTarget';

export type StatusBarPathDisplay = {
  primary: string;
  suffix?: string;
  fullPath?: string;
};

function normalizePath(path: string | null | undefined): string {
  return (path || '').trim().replace(/\/+$/, '');
}

export function formatStatusBarPathDisplay(
  workspaceDir: string | null | undefined,
  filePath: string | null | undefined,
): StatusBarPathDisplay {
  const dir = normalizePath(workspaceDir);
  const file = normalizePath(filePath);

  if (!file) {
    return { primary: dir || 'No folder' };
  }

  if (dir && isPathInsideRoot(file, dir)) {
    if (file === dir) {
      return { primary: dir, fullPath: file };
    }
    return {
      primary: dir,
      suffix: file.slice(dir.length + 1),
      fullPath: file,
    };
  }

  return { primary: file, fullPath: file };
}
