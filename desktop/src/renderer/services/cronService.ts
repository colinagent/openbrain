import type { WSConnection } from './wsConnection';

export const CRON_TASK_HISTORY_LIMIT = 99;

export type CronTaskSchedule = {
  cron?: string;
  every?: string;
  time?: string;
};

export type CronTaskTarget = {
  kind: string;
  agentID?: string;
  cwd?: string;
};

export type CronTaskPayload = {
  kind: string;
  text?: string;
  data?: Record<string, unknown>;
};

export type CronTask = {
  id: string;
  name: string;
  description?: string;
  enabled: boolean;
  schedule: CronTaskSchedule;
  target: CronTaskTarget;
  payload: CronTaskPayload;
  createdAtMs?: number;
  updatedAtMs?: number;
};

export type CronTaskState = {
  taskID: string;
  specHash?: string;
  nextRunAtMs?: number;
  runNowAtMs?: number;
  lastRunAtMs?: number;
  runningAtMs?: number;
  lastError?: string;
  consecutiveErrors?: number;
};

export type CronTaskRecord = {
  task: CronTask;
  state?: CronTaskState | null;
};

export type CronListResult = {
  version?: number;
  tasks?: CronTaskRecord[];
};

export type CronRunResult = {
  queued?: boolean;
  task?: CronTaskRecord;
};

export type CronTaskHistoryEntry = {
  runID: string;
  taskID: string;
  trigger: string;
  scheduledAtMs?: number;
  startedAtMs: number;
  finishedAtMs?: number;
  durationMs?: number;
  status: string;
  error?: string;
  threadID?: string;
  chatPath?: string;
  agentID?: string;
};

export type CronTaskHistoryResult = {
  taskID?: string;
  limit?: number;
  runs?: CronTaskHistoryEntry[];
};

class CronService {
  constructor(private connection: WSConnection) {}

  async list(): Promise<CronTaskRecord[]> {
    const result = await this.connection.request<CronListResult>('cron/list', {});
    return Array.isArray(result.tasks) ? result.tasks : [];
  }

  async get(id: string): Promise<CronTaskRecord | null> {
    if (!id.trim()) {
      return null;
    }
    const result = await this.connection.request<{ task?: CronTask; state?: CronTaskState | null }>('cron/get', { id });
    return result?.task ? { task: result.task, state: result.state || null } : null;
  }

  async update(task: CronTask): Promise<CronTaskRecord | null> {
    const result = await this.connection.request<{ task?: CronTask; state?: CronTaskState | null }>('cron/update', { task });
    return result?.task ? { task: result.task, state: result.state || null } : null;
  }

  async run(id: string): Promise<CronRunResult> {
    return this.connection.request<CronRunResult>('cron/run', { id });
  }

  async history(id: string, limit = CRON_TASK_HISTORY_LIMIT): Promise<CronTaskHistoryEntry[]> {
    if (!id.trim()) {
      return [];
    }
    const result = await this.connection.request<CronTaskHistoryResult>('cron/history', { id, limit });
    return Array.isArray(result.runs) ? result.runs : [];
  }
}

export function createCronService(connection: WSConnection): CronService {
  return new CronService(connection);
}

export type { CronService };
