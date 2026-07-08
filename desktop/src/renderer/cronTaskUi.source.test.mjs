import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function read(relativePath) {
  return readFileSync(path.join(__dirname, relativePath), 'utf8');
}

test('Cron sidebar uses task names for rows and opens a task editor tab', () => {
  const sidebarSource = read('./components/Sidebar/CronSidebar.tsx');
  const displaySource = read('./utils/cronDisplay.ts');
  const appStoreSource = read('./store/appStore.ts');

  assert.match(displaySource, /return record\.task\.name\.trim\(\) \|\| record\.task\.id;/);
  assert.doesNotMatch(sidebarSource, /taskWorkspaceName/);
  assert.match(sidebarSource, /cronTaskTitle\(record\)/);
  assert.match(sidebarSource, /cronTaskPath\(record\)/);
  assert.doesNotMatch(sidebarSource, /Schedule:/);
  assert.doesNotMatch(sidebarSource, /Last:/);
  assert.doesNotMatch(sidebarSource, /Branch:/);
  assert.match(sidebarSource, /openCronTaskTab\(record\.task\.id, cronTaskTitle\(record\)\)/);
  assert.match(appStoreSource, /openCronTaskTab: \(id: string, title\?: string\) => void/);
  assert.match(appStoreSource, /const editorId = `cron-task:\$\{taskID\}`;/);
});

test('Cron task editor is a non-file editor backed by cron\\/get', () => {
  const appSource = read('./App.tsx');
  const editorSource = read('./components/Editor/CronTaskEditor.tsx');
  const serviceSource = read('./services/cronService.ts');
  const appStoreSource = read('./store/appStore.ts');

  assert.match(appSource, /import \{ CronTaskEditor \} from '\.\/components\/Editor\/CronTaskEditor';/);
  assert.match(appSource, /tab\.editorId\.startsWith\('cron-task:'\)/);
  assert.match(editorSource, /editorId\.startsWith\('cron-task:'\)/);
  assert.match(editorSource, /getCronTask\(taskID\)/);
  assert.match(editorSource, /updateCronTask\(buildUpdatedTask/);
  assert.match(editorSource, /runCronTask\(taskID\)/);
  assert.match(editorSource, /listCronTaskHistory\(record\.task\.id, CRON_TASK_HISTORY_LIMIT\)/);
  assert.match(editorSource, /onClick=\{\(\) => setShowHistory\(\(current\) => !current\)\}/);
  assert.match(editorSource, /getThreadMeta\(/);
  assert.match(editorSource, /if \(meta\) \{\s*chatStore\.upsertThreadMeta\(meta\);\s*\}/);
  assert.match(editorSource, /openThreadConversation\(threadID, \{/);
  assert.match(editorSource, /meta\?\.chatPath \|\| run\.chatPath/);
  assert.match(editorSource, /title: meta\?\.title \|\| threadID \|\| 'Cron run'/);
  assert.doesNotMatch(editorSource, /openComposerThreadTarget/);
  assert.doesNotMatch(editorSource, /setActivityExpanded\(threadID/);
  assert.doesNotMatch(editorSource, /refreshThreadStateByThreadID/);
  assert.doesNotMatch(editorSource, /run\.runID \|\| 'Cron run'/);
  assert.doesNotMatch(editorSource, /getThreadSnapshot/);
  assert.doesNotMatch(editorSource, /openScratchTab/);
  assert.doesNotMatch(editorSource, /Snapshot/);
  assert.match(editorSource, /nameMode: name === defaultName \? 'auto' : 'custom'/);
  assert.match(editorSource, /Reset name/);
  assert.match(serviceSource, /'cron\/get'/);
  assert.match(serviceSource, /'cron\/update'/);
  assert.match(serviceSource, /'cron\/history'/);
  assert.match(appStoreSource, /updateCronTask: \(task: CronTask\) => Promise<CronTaskRecord>/);
  assert.match(appStoreSource, /listCronTaskHistory: \(id: string, limit\?: number\) => Promise<CronTaskHistoryEntry\[]>/);
  assert.doesNotMatch(appStoreSource, /openScratchTab/);
  assert.match(editorSource, /type="number"/);
  assert.match(editorSource, /Interval \(minutes\)/);
  assert.doesNotMatch(editorSource, /Quick Interval/);
  assert.match(editorSource, /<select/);
  assert.match(editorSource, /Model/);
  assert.match(editorSource, /modelKey: draftModelKey/);
  assert.match(editorSource, /min=\{1\}/);
  assert.match(editorSource, /Math\.max\(1, Math\.round\(draftIntervalSec \/ 60\)\)/);
  assert.match(editorSource, /normalizeCronIntervalSec\(minutes \* 60\)/);
  assert.match(editorSource, /Runs/);
  assert.match(editorSource, /Recent Runs/);
  assert.match(editorSource, /Latest 99/);
  assert.match(editorSource, /Conversation/);
  assert.doesNotMatch(editorSource, /Chat file/);
  assert.doesNotMatch(editorSource, /JSONL/);
});

test('Cron schedule utilities preserve user configured short intervals', () => {
  const source = read('./utils/cronSchedule.ts');

  assert.match(source, /\{ seconds: 60, label: '1 min' \}/);
  assert.doesNotMatch(source, /value < MIN_CRON_INTERVAL_SEC/);
  assert.match(source, /Math\.max\(1, Math\.round\(value\)\)/);
});

test('Workspace sync status control writes policy before refreshing managed cron', () => {
  const source = read('./components/WorkspaceSyncStatusControl.tsx');
  const appStoreSource = read('./store/appStore.ts');
  const serviceSource = read('./services/storageService.ts');

  assert.match(source, /listCronTasks/);
  assert.match(source, /updateWorkspaceSyncPolicy/);
  assert.match(source, /findWorkspaceCronTask/);
  assert.match(source, /saveTaskPatch/);
  assert.match(source, /intervalSec: patch\.intervalSec \?\? intervalSec/);
  assert.match(appStoreSource, /updateWorkspaceSyncPolicy: \(policy: WorkspaceSyncPolicy\) => Promise<WorkspaceStorageInfo \| null>/);
  assert.match(serviceSource, /'storage\/updatePolicy'/);
  assert.match(appStoreSource, /resolveOpenBrainCloudSyncModelParams/);
  assert.match(serviceSource, /modelKey\?: string/);
  assert.match(source, /window\.setInterval\(\(\) => \{/);
  assert.match(source, /refreshCronTask\(undefined, \{ background: true \}\)/);
});

test('Cron status labels are product-facing and avoid internal runtime wording', () => {
  const displaySource = read('./utils/cronDisplay.ts');
  const sidebarSource = read('./components/Sidebar/CronSidebar.tsx');

  assert.match(displaySource, /label: 'Ready'/);
  assert.match(displaySource, /label: 'Paused'/);
  assert.match(displaySource, /label: 'Failed'/);
  assert.doesNotMatch(displaySource, /label: 'Idle'/);
  assert.doesNotMatch(sidebarSource, /uppercase tracking-wide/);
  assert.match(sidebarSource, /rounded-full/);
  assert.match(sidebarSource, /status\.label === 'Ready'/);
  assert.match(sidebarSource, /hover:bg-hover-bg/);
  assert.match(sidebarSource, /hover:text-highlight/);
  assert.doesNotMatch(sidebarSource, /hover:bg-hover-bg hover:text-prime-text/);
});
