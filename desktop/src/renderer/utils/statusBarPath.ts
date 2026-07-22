export type StatusBarPathDisplay = {
  label: string;
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
    return { label: dir || 'No folder' };
  }

  return { label: file };
}
