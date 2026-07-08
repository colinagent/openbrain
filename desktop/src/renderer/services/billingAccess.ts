import type { ModelEntry } from '../types/electron';
import { rendererI18n } from '../../main/i18n/renderer';
import type { BillingReminderKind } from '../store/billingReminderStore';
import {
  isOpenBrainProviderKey,
  normalizeModelKey,
  normalizeProviderKey,
} from '../../shared/modelKeys';

export const BILLING_URL = 'https://openbrain.chat/en/billing';

export type DesktopBillingSubscription = {
  uid: string;
  planId?: string;
  planName?: string;
  effectivePlanId?: string;
  effectivePlanName?: string;
  effectivePlanSource?: string;
  stripeSubscriptionId?: string;
  status?: string;
  currentPeriodStart?: string;
  currentPeriodEnd?: string;
  cancelAtPeriodEnd?: boolean;
  quota?: {
    currency?: string;
    baseMonthlyCost?: string;
    giftedMonthlyCost?: string;
    grantedAdjustment?: string;
    effectiveCostQuota?: string;
    usedCost?: string;
    remainingCost?: string;
  };
  aiChatEligible?: boolean;
  bundledTokenEligible?: boolean;
};

export type ChatModelAccessResult = {
  allowed: boolean;
  message: string;
  reminderKind?: BillingReminderKind;
  authInvalid?: boolean;
};

export type ChatAccessResult =
  | { allowed: true }
  | {
      allowed: false;
      reason: 'login-required' | 'billing-required';
      message: string;
      reminderKind?: BillingReminderKind;
    };

type BillingSubscriptionResult = {
  success?: boolean;
  error?: string;
  authInvalid?: boolean;
  subscription?: DesktopBillingSubscription | null;
};

const BILLING_CACHE_TTL_MS = 30_000;
function chatUnavailableMessage(): string {
  return rendererI18n.t('error:billing.chatUnavailable');
}

function bundledTokenRequiredMessage(): string {
  return rendererI18n.t('error:billing.creditsRequired');
}

function quotaExhaustedMessage(): string {
  return rendererI18n.t('error:billing.quotaExhausted');
}

export type BillingRestrictionInfo = {
  reminderKind: BillingReminderKind;
  title: string;
  summary: string;
  message: string;
  actionLabel: string;
  secondaryActionLabel?: string;
};

let cachedAt = 0;
let cachedSubscription: DesktopBillingSubscription | null = null;
let inflightSubscription: Promise<DesktopBillingSubscription | null> | null = null;
let lastAuthInvalid = false;

function inferProviderFromModelKey(modelKey: string | null | undefined): string {
  const normalizedModelKey = normalizeModelKey(modelKey);
  if (!normalizedModelKey) {
    return '';
  }
  const separator = normalizedModelKey.indexOf(':');
  if (separator <= 0) {
    return '';
  }
  return normalizeProviderKey(normalizedModelKey.slice(0, separator));
}

function providerUsesBundledTokenValue(provider: string | null | undefined): boolean {
  return isOpenBrainProviderKey(provider);
}

function normalizeBillingErrorCode(value: string | null | undefined): string {
  return (value || '').trim().toLowerCase();
}

function normalizeBillingErrorMessage(value: string | null | undefined): string {
  return (value || '').trim().toLowerCase();
}

export function isQuotaExhaustedError(
  message: string | null | undefined,
  errorCode?: string | null,
): boolean {
  return normalizeBillingErrorCode(errorCode) === 'quota_exhausted'
    || /quota[_\s-]*exhausted/i.test(message || '')
    || /billing quota exhausted/i.test(message || '');
}

function isBundledTokenRequiredError(
  message: string | null | undefined,
  errorCode?: string | null,
): boolean {
  const code = normalizeBillingErrorCode(errorCode);
  if (code === 'bundled_token_required' || code === 'credits_required') {
    return true;
  }
  const normalizedMessage = normalizeBillingErrorMessage(message);
  return normalizedMessage.includes('selected model requires credits')
    || normalizedMessage.includes('requires credits');
}

function isChatUnavailableError(
  message: string | null | undefined,
  errorCode?: string | null,
): boolean {
  const code = normalizeBillingErrorCode(errorCode);
  if (code === 'chat_unavailable' || code === 'chat-unavailable') {
    return true;
  }
  return normalizeBillingErrorMessage(message).includes('does not currently have chat access');
}

export function resolveBillingRestrictionInfo(
  message: string | null | undefined,
  errorCode?: string | null,
): BillingRestrictionInfo | null {
  if (isQuotaExhaustedError(message, errorCode)) {
    return {
      reminderKind: 'quota-exhausted',
      title: rendererI18n.t('error:billing.titleQuota'),
      summary: rendererI18n.t('error:billing.summaryQuota'),
      message: quotaExhaustedMessage(),
      actionLabel: rendererI18n.t('error:billing.actionBilling'),
    };
  }
  if (isBundledTokenRequiredError(message, errorCode)) {
    return {
      reminderKind: 'bundled-token-required',
      title: rendererI18n.t('error:billing.titleCredits'),
      summary: rendererI18n.t('error:billing.summaryCredits'),
      message: bundledTokenRequiredMessage(),
      actionLabel: rendererI18n.t('error:billing.actionBilling'),
      secondaryActionLabel: rendererI18n.t('error:billing.actionModels'),
    };
  }
  if (isChatUnavailableError(message, errorCode)) {
    const unavailable = chatUnavailableMessage();
    return {
      reminderKind: 'chat-unavailable',
      title: rendererI18n.t('error:billing.titleChatUnavailable'),
      summary: unavailable,
      message: unavailable,
      actionLabel: rendererI18n.t('error:billing.actionBilling'),
    };
  }
  return null;
}

export function modelRequiresBundledTokenValue(
  modelKey: string | null | undefined,
  model: Pick<ModelEntry, 'provider'> | null | undefined,
): boolean {
  if (providerUsesBundledTokenValue(model?.provider)) {
    return true;
  }
  const inferredProvider = inferProviderFromModelKey(modelKey);
  if (inferredProvider) {
    return providerUsesBundledTokenValue(inferredProvider);
  }
  return normalizeModelKey(modelKey) !== '';
}

export function resolveChatModelAccess(
  subscription: Pick<DesktopBillingSubscription, 'aiChatEligible' | 'bundledTokenEligible'> | null | undefined,
  modelKey: string | null | undefined,
  model: Pick<ModelEntry, 'provider'> | null | undefined,
): ChatModelAccessResult | null {
  if (modelRequiresBundledTokenValue(modelKey, model) && subscription?.bundledTokenEligible !== true) {
    return {
      allowed: false,
      message: bundledTokenRequiredMessage(),
      reminderKind: 'bundled-token-required',
    };
  }
  return null;
}

export function clearBillingAccessCache(): void {
  cachedAt = 0;
  cachedSubscription = null;
  inflightSubscription = null;
  lastAuthInvalid = false;
}

export async function getDesktopBillingSubscription(force = false): Promise<DesktopBillingSubscription | null> {
  const now = Date.now();
  if (!force && !lastAuthInvalid && cachedSubscription && now - cachedAt < BILLING_CACHE_TTL_MS) {
    return cachedSubscription;
  }
  if (!force && inflightSubscription) {
    return inflightSubscription;
  }
  if (!window.electronAPI?.billing?.getSubscription) {
    return null;
  }

  inflightSubscription = window.electronAPI.billing.getSubscription()
    .then((result) => {
      const typedResult = (result || {}) as BillingSubscriptionResult;
      const error = (typedResult.error || '').trim().toLowerCase();
      if (typedResult.authInvalid || error === 'not logged in') {
        lastAuthInvalid = true;
        cachedSubscription = null;
        cachedAt = 0;
        return null;
      }
      lastAuthInvalid = false;
      const subscription = typedResult.success ? (typedResult.subscription || null) : null;
      cachedSubscription = subscription;
      cachedAt = Date.now();
      return subscription;
    })
    .finally(() => {
      inflightSubscription = null;
    });

  return inflightSubscription;
}

export async function requireChatAccess(): Promise<boolean> {
  const result = await checkChatAccess();
  return result.allowed;
}

export async function checkChatAccess(): Promise<ChatAccessResult> {
  const subscription = await getDesktopBillingSubscription();
  if (lastAuthInvalid) {
    return {
      allowed: false,
      reason: 'login-required',
      message: rendererI18n.t('error:billing.signInFirst'),
    };
  }
  if (!subscription) {
    // The server-side chat path is the authority for hosted-model credit checks.
    // If this desktop preflight cannot verify billing for a non-auth reason, do
    // not block users from chatting with their own provider models.
    return { allowed: true };
  }
  return { allowed: true };
}

export async function getChatModelAccess(
  modelKey: string | null | undefined,
  model: Pick<ModelEntry, 'provider'> | null | undefined,
): Promise<ChatModelAccessResult | null> {
  const subscription = await getDesktopBillingSubscription();
  if (lastAuthInvalid) {
    return {
      allowed: false,
      message: rendererI18n.t('error:billing.signInFirst'),
      authInvalid: true,
    };
  }
  if (!subscription) {
    return null;
  }
  return resolveChatModelAccess(subscription, modelKey, model);
}
