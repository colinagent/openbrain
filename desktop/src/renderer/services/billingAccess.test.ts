import assert from 'node:assert/strict';
import test from 'node:test';

import {
  checkChatAccess,
  clearBillingAccessCache,
  getChatModelAccess,
  modelRequiresBundledTokenValue,
  requireChatAccess,
  resolveBillingRestrictionInfo,
  resolveChatModelAccess,
} from './billingAccess';

const originalWindow = globalThis.window;

function mockBillingResult(result: unknown): void {
  (globalThis as { window?: unknown }).window = {
    electronAPI: {
      billing: {
        getSubscription: async () => result,
      },
    },
  };
}

test.afterEach(() => {
  clearBillingAccessCache();
  if (originalWindow === undefined) {
    delete (globalThis as { window?: Window }).window;
    return;
  }
  (globalThis as { window?: unknown }).window = originalWindow;
});

test('requireChatAccess allows users when backend marks aiChatEligible', async () => {
  mockBillingResult({
    success: true,
    subscription: {
      uid: 'user-free-gift',
      planId: 'free',
      aiChatEligible: true,
      bundledTokenEligible: true,
      quota: {
        remainingCost: '2.50',
      },
    },
  });

  assert.equal(await requireChatAccess(), true);
});

test('requireChatAccess allows paid users when aiChatEligible is true without credits', async () => {
  mockBillingResult({
    success: true,
    subscription: {
      uid: 'user-paid-chat',
      planId: 'pro',
      stripeSubscriptionId: 'sub_eligible',
      status: 'active',
      aiChatEligible: true,
      bundledTokenEligible: false,
    },
  });

  assert.equal(await requireChatAccess(), true);
});

test('requireChatAccess allows non-auth billing preflight failures', async () => {
  mockBillingResult({
    success: false,
    error: 'temporary billing fetch failure',
  });

  assert.equal(await requireChatAccess(), true);
  assert.equal(await getChatModelAccess('cloud:gpt-5.4', { provider: 'cloud' }), null);
});

test('resolveChatModelAccess blocks Cloud models when credits are unavailable', () => {
  const result = resolveChatModelAccess(
    {
      aiChatEligible: false,
      bundledTokenEligible: false,
    },
    'cloud:gpt-5.4',
    { provider: 'cloud' },
  );

  assert.deepEqual(result, {
    allowed: false,
    message: 'This selected model requires credits. Choose a model from another provider in Models, or go to Billing to get credits.',
    reminderKind: 'bundled-token-required',
  });
});

test('resolveChatModelAccess allows non-Cloud provider models without credits', () => {
  const result = resolveChatModelAccess(
    {
      aiChatEligible: false,
      bundledTokenEligible: false,
    },
    'openai:gpt-5',
    { provider: 'openai' },
  );

  assert.equal(result, null);
  assert.equal(modelRequiresBundledTokenValue('openai:gpt-5', { provider: 'openai' }), false);
  assert.equal(modelRequiresBundledTokenValue('cloud:gpt-5', { provider: 'cloud' }), true);
  assert.equal(modelRequiresBundledTokenValue('third-party:gpt-5', null), false);
});

test('requireChatAccess allows chat even when backend reports no paid entitlement', async () => {
  mockBillingResult({
    success: true,
    subscription: {
      uid: 'user-paid-blocked',
      planId: 'pro',
      stripeSubscriptionId: 'sub_123',
      status: 'active',
      aiChatEligible: false,
      bundledTokenEligible: false,
    },
  });

  assert.equal(await requireChatAccess(), true);
});

test('checkChatAccess treats expired sessions as login-required, not billing-required', async () => {
  mockBillingResult({
    success: false,
    error: 'invalid session',
    authInvalid: true,
  });

  assert.deepEqual(await checkChatAccess(), {
    allowed: false,
    reason: 'login-required',
    message: 'Please sign in first.',
  });
  assert.deepEqual(await getChatModelAccess('cloud:gpt-5.4', { provider: 'cloud' }), {
    allowed: false,
    message: 'Please sign in first.',
    authInvalid: true,
  });
});
