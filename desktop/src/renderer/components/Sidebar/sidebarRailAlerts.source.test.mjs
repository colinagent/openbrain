import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function read(relativePath) {
  return readFileSync(path.join(__dirname, relativePath), 'utf8');
}

test('sidebar rail alert badge is a generic reusable numbered overlay', () => {
  const badgeSource = read('./SidebarRailAlertBadge.tsx');
  const alertsSource = read('./sidebarRailAlerts.ts');
  const stylesSource = read('../../styles/index.css');

  assert.match(badgeSource, /export function SidebarRailAlertBadge/);
  assert.match(badgeSource, /sidebar-rail-alert-badge/);
  assert.match(badgeSource, /formatSidebarRailAlertCount/);
  assert.match(alertsSource, /export function formatSidebarRailAlertCount/);
  assert.match(stylesSource, /--op-rail-alert:\s*var\(--op-messenger-pending\);/);
  assert.match(stylesSource, /\.sidebar-rail-alert-badge\s*\{[^}]*background:\s*var\(--op-rail-alert\);/m);
  assert.match(stylesSource, /\.sidebar-rail-alert-badge\s*\{[^}]*min-width:\s*13px;/m);
});

test('sidebar aggregates rail alerts through useSidebarRailAlerts', () => {
  const sidebarSource = read('./Sidebar.tsx');
  const hookSource = read('./useSidebarRailAlerts.ts');
  const alertsSource = read('./sidebarRailAlerts.ts');

  assert.match(sidebarSource, /useSidebarRailAlerts/);
  assert.match(sidebarSource, /SidebarRailAlertBadge/);
  assert.match(sidebarSource, /sidebarRailAlertCount\(railAlerts, item\.key\)/);
  assert.match(hookSource, /countCronBillingAlerts/);
  assert.match(alertsSource, /hasBillingRunAlert/);
  assert.match(alertsSource, /export type SidebarRailAlertMap/);
});

test('formatSidebarRailAlertCount caps display at ninety-nine plus', () => {
  const alertsSource = read('./sidebarRailAlerts.ts');
  const functionSource = alertsSource.match(
    /export function formatSidebarRailAlertCount\(count: number\): string \{[\s\S]*?\n\}/,
  );
  assert.ok(functionSource);
  const executableSource = functionSource[0]
    .replace('export ', '')
    .replace('(count: number): string', '(count)');
  const formatSidebarRailAlertCount = Function(
    `"use strict"; ${executableSource}; return formatSidebarRailAlertCount;`,
  )();

  assert.equal(formatSidebarRailAlertCount(0), '0');
  assert.equal(formatSidebarRailAlertCount(1), '1');
  assert.equal(formatSidebarRailAlertCount(99), '99');
  assert.equal(formatSidebarRailAlertCount(100), '99+');
});
