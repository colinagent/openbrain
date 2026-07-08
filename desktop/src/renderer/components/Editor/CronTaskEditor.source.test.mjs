import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

test('Cron task history UI and services use the latest 99 run window', () => {
  const editorSource = readFileSync(path.join(__dirname, 'CronTaskEditor.tsx'), 'utf8');
  const serviceSource = readFileSync(path.join(__dirname, '../../services/cronService.ts'), 'utf8');
  const appStoreSource = readFileSync(path.join(__dirname, '../../store/appStore.ts'), 'utf8');

  assert.match(serviceSource, /export const CRON_TASK_HISTORY_LIMIT = 99;/);
  assert.match(serviceSource, /async history\(id: string, limit = CRON_TASK_HISTORY_LIMIT\)/);
  assert.match(appStoreSource, /listCronTaskHistory: async \(id: string, limit = CRON_TASK_HISTORY_LIMIT\) =>/);
  assert.match(editorSource, /listCronTaskHistory\(record\.task\.id, CRON_TASK_HISTORY_LIMIT\)/);
  assert.match(editorSource, />Latest 99</);
  assert.doesNotMatch(editorSource, /Latest 100/);
});
