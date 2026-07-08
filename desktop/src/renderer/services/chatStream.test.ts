import assert from 'node:assert/strict';
import test from 'node:test';

import { ChatStreamError, streamChat } from './chatStream';

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

test('streamChat parses SSE ids and forwards Last-Event-ID', async () => {
  const originalFetch = globalThis.fetch;
  let lastEventIDHeader = '';
  try {
    globalThis.fetch = (async (_input: Parameters<typeof fetch>[0], init?: Parameters<typeof fetch>[1]) => {
      lastEventIDHeader = String((init?.headers as Record<string, string>)['Last-Event-ID'] || '');
      return new Response(
        sseBody(
          'id: 7\nevent: message\ndata: {"meta":{"type":"stream"},"content":{"text":"hello"}}\n\n',
          'id: 8\nevent: message\ndata: {"meta":{"type":"end"},"content":{"text":""}}\n\n',
        ),
        {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        },
      );
    }) as unknown as typeof fetch;

    const seen: Array<{ id: string | null; text: string }> = [];
    const result = await streamChat('http://example.test/v1/chat/stream', { ok: true }, {
      lastEventID: '6',
      onEvent: (event, meta) => {
        seen.push({ id: meta.id, text: String(event.content?.text || '') });
      },
    });

    assert.equal(lastEventIDHeader, '6');
    assert.deepEqual(seen, [
      { id: '7', text: 'hello' },
      { id: '8', text: '' },
    ]);
    assert.equal(result.lastEventID, '8');
    assert.equal(result.ended, true);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChat treats disconnect before end as retryable transport failure', async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = (async () => new Response(
      sseBody('id: 3\nevent: message\ndata: {"meta":{"type":"stream"},"content":{"text":"partial"}}\n\n'),
      {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      },
    )) as unknown as typeof fetch;

    await assert.rejects(
      () => streamChat('http://example.test/v1/chat/stream', { ok: true }, {
        onEvent: () => {},
      }),
      (error: unknown) => {
        assert.ok(error instanceof ChatStreamError);
        assert.equal(error.retryable, true);
        assert.equal(error.lastEventID, '3');
        assert.match(error.message, /disconnected before completion/i);
        return true;
      },
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChat surfaces 401 as a login-required error', async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = (async () => new Response(
      JSON.stringify({ error: 'Please sign in first.' }),
      {
        status: 401,
        statusText: 'Unauthorized',
        headers: { 'Content-Type': 'application/json' },
      },
    )) as unknown as typeof fetch;

    await assert.rejects(
      () => streamChat('http://example.test/v1/chat/stream', { ok: true }, {
        onEvent: () => {},
      }),
      (error: unknown) => {
        assert.ok(error instanceof ChatStreamError);
        assert.equal(error.status, 401);
        assert.equal(error.retryable, false);
        assert.equal(error.message, 'Please sign in first.');
        return true;
      },
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChat surfaces structured thread-state errors', async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = (async () => new Response(
      JSON.stringify({
        error: 'thread is already running',
        code: 'thread_running',
        threadID: 'thread-test',
        chatPath: '/tmp/chat.md',
      }),
      {
        status: 409,
        statusText: 'Conflict',
        headers: { 'Content-Type': 'application/json' },
      },
    )) as unknown as typeof fetch;

    await assert.rejects(
      () => streamChat('http://example.test/v1/chat/stream', { ok: true }, {
        onEvent: () => {},
      }),
      (error: unknown) => {
        assert.ok(error instanceof ChatStreamError);
        assert.equal(error.code, 'thread_running');
        assert.equal(error.threadID, 'thread-test');
        assert.equal(error.chatPath, '/tmp/chat.md');
        assert.equal(error.retryable, false);
        return true;
      },
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('streamChat falls back to error.type when code is omitted', async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = (async () => new Response(
      JSON.stringify({
        error: {
          message: 'quota_exhausted',
          type: 'quota_exhausted',
        },
      }),
      {
        status: 403,
        statusText: 'Forbidden',
        headers: { 'Content-Type': 'application/json' },
      },
    )) as unknown as typeof fetch;

    await assert.rejects(
      () => streamChat('http://example.test/v1/chat/stream', { ok: true }, {
        onEvent: () => {},
      }),
      (error: unknown) => {
        assert.ok(error instanceof ChatStreamError);
        assert.equal(error.code, 'quota_exhausted');
        assert.equal(error.message, 'Chat stream failed: 403 Forbidden - quota_exhausted');
        return true;
      },
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});
