import type { AuthConfig } from './authStore';

function normalizeAbsoluteBaseURL(raw: string): string {
  const value = raw.trim();
  if (!value) {
    return '';
  }
  try {
    const url = new URL(value);
    url.pathname = url.pathname.replace(/\/+$/, '');
    url.search = '';
    url.hash = '';
    return url.toString().replace(/\/$/, '');
  } catch {
    return value.replace(/\/+$/, '');
  }
}

export function resolveOpenBrainAPIBase(auth?: Pick<AuthConfig, 'gateway'> | null): string {
  const explicit = normalizeAbsoluteBaseURL(process.env.OPENBRAIN_API_URL || '');
  if (explicit) {
    return explicit;
  }
  const gateway = normalizeAbsoluteBaseURL(auth?.gateway || '');
  if (!gateway) {
    return '';
  }
  return gateway;
}
