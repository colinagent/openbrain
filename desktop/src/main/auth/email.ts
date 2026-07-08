export function normalizeAuthEmail(email?: string | null): string | undefined {
  let value = (email || '').trim();
  if (!value) return undefined;

  for (let i = 0; i < 2 && value.includes('%') && !value.includes('@'); i++) {
    try {
      const decoded = decodeURIComponent(value).trim();
      if (!decoded || decoded === value) break;
      value = decoded;
    } catch {
      break;
    }
  }

  return value || undefined;
}
