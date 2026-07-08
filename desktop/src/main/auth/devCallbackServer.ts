import http from 'http';
import { randomBytes } from 'crypto';
import { URL } from 'url';

type CallbackPayload = {
  token: string;
  uid: string;
  email?: string;
};

type StartResult = {
  redirectTo: string;
  state: string;
  stop: () => Promise<void>;
};

function html(body: string) {
  return `<!doctype html><html><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/><title>Opmd Login</title></head><body style="font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; padding: 24px; line-height: 1.5">${body}</body></html>`;
}

/**
 * Start a one-shot localhost callback server for dev mode.
 * This avoids macOS custom-protocol limitations when running via `electron .`.
 */
export async function startDevLoginCallbackServer(onAuth: (payload: CallbackPayload) => Promise<void>): Promise<StartResult> {
  const state = randomBytes(24).toString('hex');
  let closed = false;

  const server = http.createServer(async (req, res) => {
    try {
      const method = (req.method || 'GET').toUpperCase();
      if (method !== 'GET') {
        res.statusCode = 405;
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        res.end(html('<h2>Method not allowed</h2>'));
        return;
      }

      const u = new URL(req.url || '/', 'http://127.0.0.1');
      if (u.pathname !== '/openbrain/auth/callback') {
        res.statusCode = 404;
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        res.end(html('<h2>Not found</h2>'));
        return;
      }

      const gotState = u.searchParams.get('state') || '';
      if (!gotState || gotState !== state) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        res.end(html('<h2>Invalid state</h2><p>Please retry login from openbrain.</p>'));
        return;
      }

      const token = u.searchParams.get('token') || '';
      const uid = u.searchParams.get('uid') || '';
      const email = u.searchParams.get('email') || undefined;
      if (!token || !uid) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        res.end(html('<h2>Missing token</h2><p>Please retry login from openbrain.</p>'));
        return;
      }

      await onAuth({ token, uid, email });

      res.statusCode = 200;
      res.setHeader('Content-Type', 'text/html; charset=utf-8');
      res.end(
        html(
          '<h2>Login complete</h2><p>You can close this tab and return to openbrain.</p><script>setTimeout(()=>{ try{ window.close(); } catch {} }, 300);</script>'
        )
      );
    } catch (err) {
      res.statusCode = 500;
      res.setHeader('Content-Type', 'text/html; charset=utf-8');
      res.end(html('<h2>Login error</h2><p>Return to openbrain and retry.</p>'));
      // eslint-disable-next-line no-console
      console.error('[Auth][DevCallback] handler error:', err);
    } finally {
      // one-shot: close server after first request handled
      if (!closed) {
        closed = true;
        server.close();
      }
    }
  });

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => resolve());
  });

  const addr = server.address();
  if (!addr || typeof addr === 'string') {
    await new Promise<void>((resolve) => server.close(() => resolve()));
    throw new Error('failed to start dev callback server');
  }

  const redirectTo = `http://127.0.0.1:${addr.port}/openbrain/auth/callback?state=${encodeURIComponent(state)}`;

  return {
    redirectTo,
    state,
    stop: () =>
      new Promise<void>((resolve) => {
        if (closed) return resolve();
        closed = true;
        server.close(() => resolve());
      }),
  };
}

