import { ChatStreamError, streamChat, type GeneralContent } from './chatStream';

const STREAM_RETRY_LIMIT = 5;
const STREAM_RETRY_INITIAL_DELAY_MS = 200;
const STREAM_RETRY_BACKOFF_FACTOR = 2;

export type StreamReconnectState = {
  attempt: number;
  limit: number;
  message: string;
};

export type StreamWithReconnectOptions = {
  url: string;
  payload: unknown;
  signal: AbortSignal;
  onEvent: (event: GeneralContent) => void;
  onReconnectStateChange?: (state: StreamReconnectState | null) => void;
};

function createAbortError(): Error {
  const error = new Error('Aborted');
  error.name = 'AbortError';
  return error;
}

export function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

export function isSignalAbortError(error: unknown, signal: AbortSignal): boolean {
  return signal.aborted && isAbortError(error);
}

export function generateTurnRequestID(): string {
  const randomUUID = globalThis.crypto?.randomUUID?.bind(globalThis.crypto);
  if (randomUUID) {
    return `turn_${randomUUID().replace(/-/g, '')}`;
  }
  return `turn_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`;
}

function jitterDelayMs(delayMs: number): number {
  const factor = 0.9 + Math.random() * 0.2;
  return Math.max(1, Math.round(delayMs * factor));
}

function retryDelayMs(attempt: number): number {
  const exponent = Math.max(0, attempt - 1);
  return jitterDelayMs(STREAM_RETRY_INITIAL_DELAY_MS * (STREAM_RETRY_BACKOFF_FACTOR ** exponent));
}

function sleepWithAbort(delayMs: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) {
    return Promise.reject(createAbortError());
  }
  return new Promise<void>((resolve, reject) => {
    let timer: ReturnType<typeof setTimeout>;
    const onAbort = () => {
      clearTimeout(timer);
      signal.removeEventListener('abort', onAbort);
      reject(createAbortError());
    };
    timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, delayMs);
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

function normalizeStreamError(
  error: unknown,
  signal: AbortSignal,
  lastEventID: string | null,
): unknown {
  if (isSignalAbortError(error, signal) || !isAbortError(error)) {
    return error;
  }
  return new ChatStreamError(
    lastEventID
      ? 'Chat stream disconnected before completion.'
      : 'Chat stream ended without any events.',
    {
      retryable: true,
      lastEventID,
      cause: error,
    },
  );
}

function isRetryableStreamError(error: unknown, signal: AbortSignal): boolean {
  if (isSignalAbortError(error, signal)) {
    return false;
  }
  if (error instanceof ChatStreamError) {
    return error.retryable;
  }
  if (error instanceof TypeError) {
    return true;
  }
  return false;
}

export async function streamChatWithReconnect(options: StreamWithReconnectOptions): Promise<void> {
  let attempt = 0;
  let lastEventID: string | null = null;

  while (true) {
    try {
      const result = await streamChat(options.url, options.payload, {
        signal: options.signal,
        lastEventID,
        onEvent: (event, meta) => {
          if (meta.id) {
            lastEventID = meta.id;
          }
          options.onEvent(event);
        },
      });
      if (result.lastEventID) {
        lastEventID = result.lastEventID;
      }
      options.onReconnectStateChange?.(null);
      return;
    } catch (error) {
      const normalizedError = normalizeStreamError(error, options.signal, lastEventID);
      if (normalizedError instanceof ChatStreamError && normalizedError.lastEventID) {
        lastEventID = normalizedError.lastEventID;
      }
      if (!isRetryableStreamError(normalizedError, options.signal) || attempt >= STREAM_RETRY_LIMIT) {
        options.onReconnectStateChange?.(null);
        throw normalizedError;
      }
      attempt += 1;
      const reconnectState: StreamReconnectState = {
        attempt,
        limit: STREAM_RETRY_LIMIT,
        message: `Reconnecting... ${attempt}/${STREAM_RETRY_LIMIT}`,
      };
      options.onReconnectStateChange?.(reconnectState);
      await sleepWithAbort(retryDelayMs(attempt), options.signal);
    }
  }
}
