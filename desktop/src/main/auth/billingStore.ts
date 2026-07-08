import { AuthInvalidError, isAuthInvalidResponse, readErrorMessage } from './authErrors';
import { authFetch } from './netFetch';

export type BillingQuota = {
  currency?: string;
  baseMonthlyCost?: string;
  giftedMonthlyCost?: string;
  grantedAdjustment?: string;
  effectiveCostQuota?: string;
  usedCost?: string;
  remainingCost?: string;
};

export type BillingSubscription = {
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
  quota?: BillingQuota;
  aiChatEligible?: boolean;
  bundledTokenEligible?: boolean;
};

function normalizeText(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function parseBillingQuota(value: unknown): BillingQuota | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const quota = value as Record<string, unknown>;
  return {
    currency: normalizeText(quota.currency),
    baseMonthlyCost: normalizeText(quota.baseMonthlyCost),
    giftedMonthlyCost: normalizeText(quota.giftedMonthlyCost),
    grantedAdjustment: normalizeText(quota.grantedAdjustment),
    effectiveCostQuota: normalizeText(quota.effectiveCostQuota),
    usedCost: normalizeText(quota.usedCost),
    remainingCost: normalizeText(quota.remainingCost),
  };
}

export async function fetchBillingSubscription(
  gateway: string,
  token: string,
): Promise<BillingSubscription | null> {
  try {
    const url = `${gateway.replace(/\/$/, '')}/v1/payment/subscription`;
    const response = await authFetch(url, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      const message = await readErrorMessage(response);
      if (isAuthInvalidResponse(response.status, message)) {
        throw new AuthInvalidError(message);
      }
      console.error('[billingStore] Failed to fetch billing subscription:', response.status, message);
      return null;
    }

    const data = (await response.json()) as Record<string, unknown> | null;
    if (!data || typeof data !== 'object') {
      return null;
    }

    return {
      uid: normalizeText(data.uid) || '',
      planId: normalizeText(data.planId),
      planName: normalizeText(data.planName),
      effectivePlanId: normalizeText(data.effectivePlanId),
      effectivePlanName: normalizeText(data.effectivePlanName),
      effectivePlanSource: normalizeText(data.effectivePlanSource),
      stripeSubscriptionId: normalizeText(data.stripeSubscriptionId),
      status: normalizeText(data.status),
      currentPeriodStart: normalizeText(data.currentPeriodStart),
      currentPeriodEnd: normalizeText(data.currentPeriodEnd),
      cancelAtPeriodEnd: data.cancelAtPeriodEnd === true,
      quota: parseBillingQuota(data.quota),
      aiChatEligible: data.aiChatEligible === true,
      bundledTokenEligible: data.bundledTokenEligible === true,
    };
  } catch (err) {
    if (err instanceof AuthInvalidError) {
      throw err;
    }
    console.error('[billingStore] Error fetching billing subscription:', err);
    return null;
  }
}
