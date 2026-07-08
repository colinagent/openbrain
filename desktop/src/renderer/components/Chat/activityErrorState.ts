import type { LiveStep } from '../../store/chatWorkspaceStore';
import { resolveBillingRestrictionInfo, type BillingRestrictionInfo } from '../../services/billingAccess';

export { BILLING_URL } from '../../services/billingAccess';

export type ActivityErrorInfo = Omit<BillingRestrictionInfo, 'reminderKind'> & {
  kind: BillingRestrictionInfo['reminderKind'];
};

export function resolveActivityErrorInfo(
  errorMessage: string | null | undefined,
  errorCode: string | null | undefined,
): ActivityErrorInfo | null {
  const message = (errorMessage || '').trim();
  if (!message) {
    return null;
  }
  const billingInfo = resolveBillingRestrictionInfo(message, errorCode);
  if (!billingInfo) {
    return null;
  }
  return {
    kind: billingInfo.reminderKind,
    title: billingInfo.title,
    summary: billingInfo.summary,
    message: billingInfo.message,
    actionLabel: billingInfo.actionLabel,
    secondaryActionLabel: billingInfo.secondaryActionLabel,
  };
}

export function shouldKeepActivityPanelExpandedAfterRun(
  latestStep: LiveStep | undefined,
  errorInfo: ActivityErrorInfo | null,
): boolean {
  return Boolean(errorInfo) || latestStep?.type === 'notice';
}

export type RunErrorDisplay = {
  billingInfo: ActivityErrorInfo | null;
  rawError: string;
};

/**
 * Resolve a raw cron/run error string into a user-facing display object.
 *
 * Cron run history and `state.lastError` carry the raw Go runtime call chain
 * (e.g. `failed to call agent: calling "agents/call": ... quota_exhausted`).
 * `billingInfo` is non-null when the message classifies as a billing error
 * (quota exhausted, bundled token required, chat unavailable) and the caller
 * should render `BillingRestrictionCard` instead of the raw string. `rawError`
 * is always the trimmed original string, for a "show details" fallback.
 */
export function resolveRunErrorDisplay(error: string | null | undefined): RunErrorDisplay {
  const rawError = (error || '').trim();
  return {
    billingInfo: resolveActivityErrorInfo(rawError, undefined),
    rawError,
  };
}

/** True when a raw run/cron error should surface a billing attention dot or card. */
export function hasBillingRunAlert(error: string | null | undefined): boolean {
  return resolveRunErrorDisplay(error).billingInfo !== null;
}
