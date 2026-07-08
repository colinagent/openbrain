export class AuthInvalidError extends Error {
  constructor(message = 'invalid session') {
    super(message);
    this.name = 'AuthInvalidError';
  }
}

export function isAuthInvalidError(error: unknown): error is AuthInvalidError {
  return error instanceof AuthInvalidError || (
    error instanceof Error && error.name === 'AuthInvalidError'
  );
}

export function isAuthInvalidResponse(status: number, message?: string): boolean {
  if (status !== 401 && status !== 403) {
    return false;
  }
  const normalized = (message || '').trim().toLowerCase();
  if (!normalized) {
    return true;
  }
  return normalized === 'invalid session'
    || normalized === 'invalid session uid'
    || normalized === 'missing authorization'
    || normalized === 'authentication required'
    || normalized === 'unauthorized'
    || normalized.includes('invalid session');
}

export async function readErrorMessage(response: Response): Promise<string> {
  const fallback = response.statusText || `HTTP ${response.status}`;
  try {
    const data = await response.json() as unknown;
    if (data && typeof data === 'object' && 'error' in data) {
      const message = String((data as { error?: unknown }).error || '').trim();
      return message || fallback;
    }
  } catch {
    // Non-JSON error bodies are fine; callers only need a best-effort message.
  }
  return fallback;
}
