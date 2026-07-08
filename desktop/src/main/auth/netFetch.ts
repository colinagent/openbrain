import { net } from 'electron';

export async function authFetch(
  input: string,
  init?: RequestInit,
): Promise<Response> {
  return net.fetch(input, init);
}

export function readableNetworkError(err: unknown): string {
  if (err instanceof Error && err.message.trim()) {
    return err.message.trim();
  }
  return String(err || 'network error');
}
