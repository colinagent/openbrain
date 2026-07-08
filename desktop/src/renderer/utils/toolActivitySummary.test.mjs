import assert from 'node:assert/strict';
import test from 'node:test';

import { getToolActivityDetailPreview } from './toolActivitySummary.ts';

test('tool activity detail preview uses the first non-empty line', () => {
  assert.equal(getToolActivityDetailPreview('\n  bash: pwd\n\n$ pwd'), 'bash: pwd');
  assert.equal(getToolActivityDetailPreview('\n\n'), '');
});
