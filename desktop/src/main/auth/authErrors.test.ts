import assert from 'node:assert/strict';
import test from 'node:test';
import { isAuthInvalidResponse } from './authErrors';

test('auth invalidation recognizes revoked sessions without treating tenant denial as global logout', () => {
  assert.equal(isAuthInvalidResponse(401, 'session revoked'), true);
  assert.equal(isAuthInvalidResponse(403, 'Session revoked by administrator'), true);
  assert.equal(isAuthInvalidResponse(403, 'tenant access denied'), false);
  assert.equal(isAuthInvalidResponse(500, 'session revoked'), false);
});
