import assert from 'node:assert/strict';
import test from 'node:test';

import { streamChatWithReconnect } from './chatStreamRetry';

function sseBody(...events: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const event of events) {
        controller.enqueue(encoder.encode(event));
      }
      controller.close();
    },
  });
}

test('streamChatWithReconnect retries once and resumes from Last-Event-ID', async () => {
  const originalFetch = globalThis.fetch;
  try {
    const requestLastEventIDs: string[] = [];
    let callCount = 0;
    globalThis.fetch = (async (_input: Parameters<typeof fetch>[0], init?: Parameters<typeof fetch>[1]) => {
      callCount += 1;
      const headers = (init?.headers as Record<string, string>) || {};
      requestLastEventIDs.push(String(headers['Last-Event-ID'] || ''));
      if (callCount === 1) {
        return new Response(
          sseBody('id: 1\nevent: message\ndata: {"meta":{"type":"stream"},"content":{"text":"hello"}}\n\n'),
          {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          },
        );
      }
      return new Response(
        sseBody('id: 2\nevent: message\ndata: {"meta":{"type":"end"},"content":{"text":""}}\n\n'),
        {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        },
      );
    }) as unknown as typeof fetch;

    const reconnectStates: Array<string | null> = [];
    const seenTexts: string[] = [];
    await streamChatWithReconnect({
      url: 'http://example.test/v1/chat/stream',
      payload: { ok: true },
      signal: new AbortController().signal,
      onEvent: (event) => {
        seenTexts.push(String(event.content?.text || ''));
      },
      onReconnectStateChange: (state) => {
        reconnectStates.push(state?.message || null);
      },
    });

    assert.deepEqual(requestLastEventIDs, ['', '1']);
    assert.deepEqual(seenTexts, ['hello', '']);
    assert.deepEqual(reconnectStates, ['Reconnecting... 1/5', null]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChatWithReconnect does not retry deterministic HTTP failures', async () => {
  const originalFetch = globalThis.fetch;
  try {
    let callCount = 0;
    globalThis.fetch = (async () => {
      callCount += 1;
      return new Response('busy', { status: 409, statusText: 'Conflict' });
    }) as unknown as typeof fetch;

    await assert.rejects(
      () => streamChatWithReconnect({
        url: 'http://example.test/v1/chat/stream',
        payload: { ok: true },
        signal: new AbortController().signal,
        onEvent: () => {},
      }),
      /409 Conflict/,
    );
    assert.equal(callCount, 1);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChatWithReconnect retries AbortError when signal was not aborted', async () => {
  const originalFetch = globalThis.fetch;
  try {
    const reconnectStates: Array<string | null> = [];
    let callCount = 0;
    globalThis.fetch = (async () => {
      callCount += 1;
      if (callCount === 1) {
        const error = new Error('The operation was aborted.');
        error.name = 'AbortError';
        throw error;
      }
      return new Response(
        sseBody('id: 2\nevent: message\ndata: {"meta":{"type":"end"},"content":{"text":""}}\n\n'),
        {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        },
      );
    }) as unknown as typeof fetch;

    await streamChatWithReconnect({
      url: 'http://example.test/v1/chat/stream',
      payload: { ok: true },
      signal: new AbortController().signal,
      onEvent: () => {},
      onReconnectStateChange: (state) => {
        reconnectStates.push(state?.message || null);
      },
    });

    assert.equal(callCount, 2);
    assert.deepEqual(reconnectStates, ['Reconnecting... 1/5', null]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChatWithReconnect does not retry AbortError after caller aborts the signal', async () => {
  const originalFetch = globalThis.fetch;
  try {
    let callCount = 0;
    globalThis.fetch = (async (_input: Parameters<typeof fetch>[0], init?: Parameters<typeof fetch>[1]) => {
      callCount += 1;
      const signal = init?.signal as AbortSignal | undefined;
      if (signal?.aborted) {
        const error = new Error('The operation was aborted.');
        error.name = 'AbortError';
        throw error;
      }
      throw new Error('expected aborted signal');
    }) as unknown as typeof fetch;

    const controller = new AbortController();
    controller.abort();

    await assert.rejects(
      () => streamChatWithReconnect({
        url: 'http://example.test/v1/chat/stream',
        payload: { ok: true },
        signal: controller.signal,
        onEvent: () => {},
      }),
      (error: unknown) => {
        assert.ok(error instanceof Error);
        assert.equal(error.name, 'AbortError');
        return true;
      },
    );
    assert.equal(callCount, 1);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChatWithReconnect stops after five retries', async () => {
  const originalFetch = globalThis.fetch;
  const originalSetTimeout = globalThis.setTimeout;
  const originalClearTimeout = globalThis.clearTimeout;
  try {
    let callCount = 0;
    globalThis.fetch = (async () => {
      callCount += 1;
      return new Response(
        sseBody('id: 1\nevent: message\ndata: {"meta":{"type":"stream"},"content":{"text":"partial"}}\n\n'),
        {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        },
      );
    }) as unknown as typeof fetch;

    globalThis.setTimeout = ((fn: (...args: any[]) => void) => {
      fn();
      return 0 as unknown as ReturnType<typeof setTimeout>;
    }) as typeof setTimeout;
    globalThis.clearTimeout = (() => {}) as typeof clearTimeout;

    await assert.rejects(
      () => streamChatWithReconnect({
        url: 'http://example.test/v1/chat/stream',
        payload: { ok: true },
        signal: new AbortController().signal,
        onEvent: () => {},
      }),
      /disconnected before completion/i,
    );
    assert.equal(callCount, 6);
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
  }
});
