import { setTimeout as delay } from 'node:timers/promises';
import WebSocket from 'ws';

export type RuntimeSystemConfig = {
  baseDir: string;
  defaultWorkspace: string;
  hostID?: string;
  [key: string]: unknown;
};

function normalizeRequiredString(value: unknown, field: string): string {
  const normalized = typeof value === 'string' ? value.trim() : '';
  if (!normalized) {
    throw new Error(`Runtime config is missing ${field}`);
  }
  return normalized;
}

export function normalizeRuntimeSystemConfig(value: unknown): RuntimeSystemConfig {
  if (!value || typeof value !== 'object') {
    throw new Error('Runtime returned an invalid system config');
  }
  const raw = value as Record<string, unknown>;
  return {
    ...raw,
    baseDir: normalizeRequiredString(raw.baseDir, 'baseDir'),
    defaultWorkspace: normalizeRequiredString(raw.defaultWorkspace, 'defaultWorkspace'),
    ...(typeof raw.hostID === 'string' && raw.hostID.trim() ? { hostID: raw.hostID.trim() } : {}),
  };
}

export async function requestRuntimeSystemConfig(
  endpointUrl: string,
  timeoutMs = 5_000,
): Promise<RuntimeSystemConfig> {
  const normalizedUrl = endpointUrl.trim();
  if (!normalizedUrl) {
    throw new Error('Runtime WebSocket endpoint is required');
  }

  return new Promise<RuntimeSystemConfig>((resolve, reject) => {
    const ws = new WebSocket(normalizedUrl);
    const requestID = 1;
    let settled = false;

    const finish = (error?: Error, result?: RuntimeSystemConfig) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timeout);
      try {
        ws.close();
      } catch {
        // Ignore close failures after the request has settled.
      }
      if (error) {
        reject(error);
      } else if (result) {
        resolve(result);
      } else {
        reject(new Error('Runtime returned an empty system config'));
      }
    };

    const timeout = setTimeout(() => {
      finish(new Error(`Runtime system config request timed out after ${timeoutMs}ms`));
    }, timeoutMs);

    ws.on('open', () => {
      ws.send(JSON.stringify({
        jsonrpc: '2.0',
        id: requestID,
        method: 'config/system/get',
        params: {},
      }));
    });

    ws.on('message', (data) => {
      try {
        const payload = JSON.parse(data.toString()) as {
          id?: number;
          result?: unknown;
          error?: { message?: string };
        };
        if (payload.id !== requestID) {
          return;
        }
        if (payload.error) {
          finish(new Error(payload.error.message || 'Runtime system config request failed'));
          return;
        }
        finish(undefined, normalizeRuntimeSystemConfig(payload.result));
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)));
      }
    });

    ws.on('error', (error) => {
      finish(error instanceof Error ? error : new Error(String(error)));
    });

    ws.on('close', () => {
      if (!settled) {
        finish(new Error('Runtime closed the system config connection before responding'));
      }
    });
  });
}

export async function waitForRuntimeSystemConfig(
  endpointUrl: string,
  options: { attempts?: number; intervalMs?: number; requestTimeoutMs?: number } = {},
): Promise<RuntimeSystemConfig> {
  const attempts = Math.max(1, options.attempts ?? 12);
  const intervalMs = Math.max(0, options.intervalMs ?? 250);
  const requestTimeoutMs = Math.max(250, options.requestTimeoutMs ?? 1_500);
  let lastError: Error | null = null;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await requestRuntimeSystemConfig(endpointUrl, requestTimeoutMs);
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));
      if (attempt + 1 < attempts && intervalMs > 0) {
        await delay(intervalMs);
      }
    }
  }
  throw lastError || new Error('Runtime system config is unavailable');
}
