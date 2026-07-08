import assert from 'node:assert/strict';
import test from 'node:test';

import type { LiveStep } from '../../store/chatWorkspaceStore';
import {
  hasBillingRunAlert,
  resolveActivityErrorInfo,
  resolveRunErrorDisplay,
  shouldKeepActivityPanelExpandedAfterRun,
} from './activityErrorState';

function createStep(overrides: Partial<LiveStep> = {}): LiveStep {
  return {
    id: overrides.id || 'step-1',
    type: overrides.type || 'reasoning',
    label: overrides.label || 'Thinking',
    status: overrides.status || 'done',
    detail: overrides.detail,
    toolOutput: overrides.toolOutput,
    ts: overrides.ts ?? 1,
  };
}

test('resolveActivityErrorInfo classifies quota_exhausted using the explicit error code', () => {
  const info = resolveActivityErrorInfo('quota_exhausted', 'quota_exhausted');

  assert.ok(info);
  assert.equal(info?.kind, 'quota-exhausted');
  assert.equal(info?.summary, 'AI quota exhausted. Open Billing to continue.');
});

test('resolveActivityErrorInfo classifies the nested cron runtime call chain as quota-exhausted without an error code', () => {
  const info = resolveActivityErrorInfo(
    'failed to call agent: calling "agents/call": calling "node/operation": quota_exhausted',
    undefined,
  );

  assert.ok(info);
  assert.equal(info?.kind, 'quota-exhausted');
});

test('resolveRunErrorDisplay returns billingInfo and the trimmed raw error for cron quota failures', () => {
  const result = resolveRunErrorDisplay(
    '  failed to call agent: calling "agents/call": calling "node/operation": quota_exhausted  ',
  );

  assert.equal(result.billingInfo?.kind, 'quota-exhausted');
  assert.equal(
    result.rawError,
    'failed to call agent: calling "agents/call": calling "node/operation": quota_exhausted',
  );
});

test('resolveRunErrorDisplay leaves non-billing errors unclassified and preserves the raw text', () => {
  const result = resolveRunErrorDisplay('calling "node/operation": file does not exist');

  assert.equal(result.billingInfo, null);
  assert.equal(result.rawError, 'calling "node/operation": file does not exist');
});

test('resolveRunErrorDisplay handles empty input', () => {
  const result = resolveRunErrorDisplay('');

  assert.equal(result.billingInfo, null);
  assert.equal(result.rawError, '');
});

test('hasBillingRunAlert detects nested cron quota errors', () => {
  assert.equal(
    hasBillingRunAlert('failed to call agent: calling "agents/call": calling "node/operation": quota_exhausted'),
    true,
  );
  assert.equal(hasBillingRunAlert('calling "node/operation": file does not exist'), false);
});

test('shouldKeepActivityPanelExpandedAfterRun keeps quota errors expanded', () => {
  const info = resolveActivityErrorInfo('quota_exhausted', 'quota_exhausted');

  assert.equal(shouldKeepActivityPanelExpandedAfterRun(undefined, info), true);
});

test('shouldKeepActivityPanelExpandedAfterRun keeps compaction notices expanded but not generic runs', () => {
  assert.equal(shouldKeepActivityPanelExpandedAfterRun(createStep({ type: 'notice' }), null), true);
  assert.equal(shouldKeepActivityPanelExpandedAfterRun(createStep({ type: 'toolcall' }), null), false);
});
