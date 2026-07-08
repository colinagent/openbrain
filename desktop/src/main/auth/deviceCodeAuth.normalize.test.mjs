import assert from 'node:assert/strict';
import test from 'node:test';
import ts from 'typescript';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.join(__dirname, 'deviceCodeAuth.ts');
const source = readFileSync(sourcePath, 'utf8')
  .replace(/import .* from 'electron';\n/, '')
  .replace(/import .* from '\.\/netFetch';\n/, '');
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;

const module = { exports: {} };
vm.runInNewContext(compiled, {
  module,
  exports: module.exports,
  console,
  Date,
  Promise,
  URL,
  setTimeout,
  clearTimeout,
});

const { deviceVerificationLoginUri, normalizeDeviceTokenResponse } = module.exports;

test('normalizes OAuth-style device token responses into desktop auth fields', () => {
  const normalized = normalizeDeviceTokenResponse({
    access_token: ' token-123 ',
    user: {
      id: ' uid-123 ',
      email: ' shadowflow123123@gmail.com ',
    },
    base_url: 'https://app.openbrain.chat',
    gateway_url: 'https://api.op-agent.com',
    ai_gateway: 'https://api.op-agent.com',
    default_org: {
      id: 'cloud',
      name: 'Cloud',
    },
  });
  assert.equal(normalized.token, 'token-123');
  assert.equal(normalized.uid, 'uid-123');
  assert.equal(normalized.email, 'shadowflow123123@gmail.com');
  assert.equal(normalized.baseUrl, 'https://app.openbrain.chat');
  assert.equal(normalized.gateway, 'https://api.op-agent.com');
  assert.equal(normalized.aiGateway, 'https://api.op-agent.com');
  assert.equal(normalized.defaultOrg?.id, 'cloud');
  assert.equal(normalized.defaultOrg?.name, 'Cloud');
});

test('rejects completed device authorization without token and uid', () => {
  assert.throws(
    () => normalizeDeviceTokenResponse({ access_token: 'token-only' }),
    /returned no uid/,
  );
  assert.throws(
    () => normalizeDeviceTokenResponse({ user: { id: 'uid-only' } }),
    /returned no token/,
  );
});

test('opens device verification through login to avoid stale browser sessions', () => {
  assert.equal(
    deviceVerificationLoginUri('https://app.openbrain.chat/device'),
    'https://app.openbrain.chat/login?redirectTo=%2Fdevice',
  );
  assert.equal(
    deviceVerificationLoginUri('https://app.openbrain.chat/device?x=1#code'),
    'https://app.openbrain.chat/login?redirectTo=%2Fdevice%3Fx%3D1%23code',
  );
});
