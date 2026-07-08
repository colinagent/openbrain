import assert from 'node:assert/strict';
import test from 'node:test';
import ts from 'typescript';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.join(__dirname, 'authStore.ts');
const source = readFileSync(sourcePath, 'utf8')
  .replace(/import \* as fs from 'fs\/promises';\n/, 'const fs = {};\n')
  .replace(/import \* as path from 'path';\n/, "const path = { join: (...parts) => parts.join('/') };\n")
  .replace(/import \{ normalizeAuthEmail \} from '\.\/email';\n/, "const normalizeAuthEmail = (raw) => { const value = (raw || '').trim().toLowerCase(); return value || undefined; };\n")
  .replace(/import \{ writeJsonFileAtomic \} from '\.\.\/shared\/jsonFile';\n/, 'const writeJsonFileAtomic = async () => {};\n');

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
  URL,
  URLSearchParams,
  Date,
});

const { parseAuthCallbackUrl, getLoginUrl } = module.exports;

test('parses openbrain hash auth callback from the web app', () => {
  const parsed = parseAuthCallbackUrl(
    'openbrain://auth/callback#token=t-123&uid=u-123&email=ShadowFlow123123%40GMAIL.com&baseUrl=https%3A%2F%2Fapp.openbrain.chat&gateway=https%3A%2F%2Fapi.op-agent.com&defaultOrgID=cloud&defaultOrgName=Cloud'
  );
  assert.equal(parsed?.token, 't-123');
  assert.equal(parsed?.uid, 'u-123');
  assert.equal(parsed?.email, 'shadowflow123123@gmail.com');
  assert.equal(parsed?.baseUrl, 'https://app.openbrain.chat');
  assert.equal(parsed?.gateway, 'https://api.op-agent.com');
  assert.equal(parsed?.aiGateway, undefined);
  assert.equal(parsed?.defaultOrgID, 'cloud');
  assert.equal(parsed?.defaultOrgName, 'Cloud');
});

test('parses openbrain query auth callback fallback', () => {
  const parsed = parseAuthCallbackUrl(
    'openbrain://auth/callback?token=t-456&uid=u-456&email=user%40example.com'
  );
  assert.equal(parsed?.token, 't-456');
  assert.equal(parsed?.uid, 'u-456');
  assert.equal(parsed?.email, 'user@example.com');
});

test('rejects legacy opagent auth callback URLs', () => {
  const parsed = parseAuthCallbackUrl(
    `${'opagent'}://auth/callback#token=t-legacy&uid=u-legacy&email=user%40example.com`
  );
  assert.equal(parsed, null);
});

test('web login fallback uses the OpenBrain protocol', () => {
  assert.equal(
    getLoginUrl('https://app.openbrain.chat/some/path'),
    'https://app.openbrain.chat/login?redirectTo=openbrain%3A%2F%2Fauth%2Fcallback'
  );
});
