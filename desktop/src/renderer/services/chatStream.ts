export type GeneralContent = {
  meta?: Record<string, any>;
  content?: {
    type?: string;
    text?: string;
    message?: any;
    payload?: any;
    [key: string]: any;
  };
};

export type StreamEventMeta = {
  id: string | null;
  eventName: string | null;
};

export type StreamChatResult = {
  eventCount: number;
  lastEventID: string | null;
  ended: boolean;
};

export class ChatStreamError extends Error {
  retryable: boolean;
  status: number | null;
  lastEventID: string | null;
  code: string | null;
  threadID: string | null;
  chatPath: string | null;

  constructor(
    message: string,
    options?: {
      retryable?: boolean;
      status?: number | null;
      lastEventID?: string | null;
      code?: string | null;
      threadID?: string | null;
      chatPath?: string | null;
      cause?: unknown;
    },
  ) {
    super(message, options?.cause ? { cause: options.cause } : undefined);
    this.name = 'ChatStreamError';
    this.retryable = Boolean(options?.retryable);
    this.status = options?.status ?? null;
    this.lastEventID = options?.lastEventID ?? null;
    this.code = options?.code ?? null;
    this.threadID = options?.threadID ?? null;
    this.chatPath = options?.chatPath ?? null;
  }
}

type StreamOptions = {
  signal?: AbortSignal;
  lastEventID?: string | null;
  onEvent: (event: GeneralContent, meta: StreamEventMeta) => void;
};

function isRetryableStatus(status: number): boolean {
  return status >= 500 || status === 429;
}

export async function streamChat(
  url: string,
  payload: unknown,
  options: StreamOptions
): Promise<StreamChatResult> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  const normalizedLastEventID = (options.lastEventID || '').trim();
  if (normalizedLastEventID) {
    headers['Last-Event-ID'] = normalizedLastEventID;
  }

  const res = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(payload),
    signal: options.signal,
  });

  if (!res.ok) {
    const errorText = (await res.text().catch(() => '')).trim();
    let errorMessage = '';
    let errorCode: string | null = null;
    let threadID: string | null = null;
    let chatPath: string | null = null;
    if (errorText.startsWith('{')) {
      try {
        const parsed = JSON.parse(errorText) as {
          error?: string | { message?: string; type?: string; code?: string };
          code?: string;
          threadID?: string;
          chatPath?: string;
        };
        if (typeof parsed.error === 'string') {
          errorMessage = parsed.error.trim();
        } else if (parsed.error && typeof parsed.error.message === 'string') {
          errorMessage = parsed.error.message.trim();
        }
        errorCode = (parsed.code || '').trim()
          || (typeof parsed.error === 'object' && parsed.error ? (parsed.error.code || '').trim() : '')
          || (typeof parsed.error === 'object' && parsed.error ? (parsed.error.type || '').trim() : '')
          || null;
        threadID = (parsed.threadID || '').trim() || null;
        chatPath = (parsed.chatPath || '').trim() || null;
      } catch {
        // ignore non-standard error bodies
      }
    }
    if (res.status === 401) {
      throw new ChatStreamError('Please sign in first.', { status: 401 });
    }
    const suffixSource = errorMessage || errorText;
    const suffix = suffixSource ? ` - ${suffixSource}` : '';
    throw new ChatStreamError(
      `Chat stream failed: ${res.status} ${res.statusText}${suffix}`,
      {
        retryable: isRetryableStatus(res.status),
        status: res.status,
        lastEventID: normalizedLastEventID || null,
        code: errorCode,
        threadID,
        chatPath,
      },
    );
  }
  if (!res.body) {
    throw new ChatStreamError('Chat stream failed: empty response body', {
      retryable: true,
      status: res.status,
      lastEventID: normalizedLastEventID || null,
    });
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let eventCount = 0;
  let lastEventID = normalizedLastEventID || null;
  let ended = false;

  const processSseEvent = (rawEvent: string) => {
    if (!rawEvent.trim()) {
      return;
    }
    const dataLines: string[] = [];
    let eventID: string | null = null;
    let eventName: string | null = null;
    const lines = rawEvent.split('\n');
    for (const rawLine of lines) {
      if (!rawLine || rawLine.startsWith(':')) {
        continue;
      }
      if (rawLine.startsWith('id:')) {
        eventID = rawLine.slice(3).trimStart().trim() || null;
        continue;
      }
      if (rawLine.startsWith('event:')) {
        eventName = rawLine.slice(6).trimStart().trim() || null;
        continue;
      }
      if (rawLine.startsWith('data:')) {
        dataLines.push(rawLine.slice(5).trimStart());
      }
    }
    if (dataLines.length === 0) {
      return;
    }
    const data = dataLines.join('\n').trim();
    if (!data) {
      return;
    }
    let parsed: GeneralContent;
    try {
      parsed = JSON.parse(data) as GeneralContent;
    } catch {
      return;
    }
    eventCount += 1;
    if (eventID) {
      lastEventID = eventID;
    }
    if (((parsed.meta?.type ?? '') as string).trim() === 'end') {
      ended = true;
    }
    options.onEvent(parsed, { id: eventID, eventName });
  };

  while (true) {
    const { value, done } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n');
    const events = buffer.split('\n\n');
    buffer = events.pop() || '';
    for (const rawEvent of events) {
      processSseEvent(rawEvent);
    }
  }

  const trailing = buffer.trim();
  if (trailing) {
    processSseEvent(trailing);
  }

  if (!ended) {
    throw new ChatStreamError(
      eventCount === 0
        ? 'Chat stream ended without any events.'
        : 'Chat stream disconnected before completion.',
      {
        retryable: true,
        status: res.status,
        lastEventID,
      },
    );
  }

  return {
    eventCount,
    lastEventID,
    ended,
  };
}
