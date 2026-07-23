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
    'openbrain://auth/callback#token=t-123&uid=u-123&email=ShadowFlow123123%40GMAIL.com&baseUrl=https%3A%2F%2Fapp.openbrain.chat&gateway=https%3A%2F%2Fapi.op-agent.com&deploymentId=dep-saas&orgId=org-acme&identityId=idn-123&connectionId=conn-123&authMethod=email&assurance=mfa&authTime=2026-07-23T00%3A00%3A00Z&expiresAt=2026-07-24T00%3A00%3A00Z'
  );
  assert.equal(parsed?.token, 't-123');
  assert.equal(parsed?.uid, 'u-123');
  assert.equal(parsed?.email, 'shadowflow123123@gmail.com');
  assert.equal(parsed?.baseUrl, 'https://app.openbrain.chat');
  assert.equal(parsed?.gateway, 'https://api.op-agent.com');
  assert.equal(parsed?.aiGateway, undefined);
  assert.equal(parsed?.deploymentID, 'dep-saas');
  assert.equal(parsed?.orgID, 'org-acme');
  assert.equal(parsed?.identityID, 'idn-123');
  assert.equal(parsed?.connectionID, 'conn-123');
  assert.equal(parsed?.authMethod, 'email');
  assert.equal(parsed?.assurance, 'mfa');
  assert.equal(parsed?.authTime, '2026-07-23T00:00:00Z');
  assert.equal(parsed?.expiresAt, '2026-07-24T00:00:00Z');
});

test('parses openbrain query auth callback fallback', () => {
  const parsed = parseAuthCallbackUrl(
    'openbrain://auth/callback?token=t-456&uid=u-456&email=user%40example.com&deploymentId=dep-private&orgId=org-team&identityId=idn-456&connectionId=conn-456&authMethod=oidc&authTime=2026-07-23T01%3A00%3A00Z&expiresAt=2026-07-23T09%3A00%3A00Z'
  );
  assert.equal(parsed?.token, 't-456');
  assert.equal(parsed?.uid, 'u-456');
  assert.equal(parsed?.email, 'user@example.com');
  assert.equal(parsed?.deploymentID, 'dep-private');
  assert.equal(parsed?.orgID, 'org-team');
});

test('rejects callbacks without tenant-bound session context', () => {
  const parsed = parseAuthCallbackUrl(
    'openbrain://auth/callback?token=t-456&uid=u-456&email=user%40example.com'
  );
  assert.equal(parsed, null);
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
