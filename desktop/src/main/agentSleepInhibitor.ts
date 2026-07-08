import type { IdleSleepPolicy } from './settings/settingsStore';
import type { PowerSaveBlocker } from 'electron';

export type PowerSaveBlockerAPI = Pick<PowerSaveBlocker, 'start' | 'stop' | 'isStarted'>;

type AgentSleepInhibitorOptions = {
  powerSaveBlocker: PowerSaveBlockerAPI;
};

export class AgentSleepInhibitorController {
  private readonly powerSaveBlocker: PowerSaveBlockerAPI;
  private policy: IdleSleepPolicy = 'off';
  private appSessionActive = false;
  private readonly windowRunning = new Map<number, boolean>();
  private blockerId: number | null = null;

  constructor(options: AgentSleepInhibitorOptions) {
    this.powerSaveBlocker = options.powerSaveBlocker;
  }

  setPolicy(policy: IdleSleepPolicy): void {
    if (this.policy === policy) {
      this.syncBlocker();
      return;
    }
    this.policy = policy;
    this.syncBlocker();
  }

  setAppSessionActive(active: boolean): void {
    if (this.appSessionActive === active) {
      this.syncBlocker();
      return;
    }
    this.appSessionActive = active;
    this.syncBlocker();
  }

  setWindowRunning(webContentsId: number, running: boolean): void {
    if (running) {
      this.windowRunning.set(webContentsId, true);
    } else {
      this.windowRunning.delete(webContentsId);
    }
    this.syncBlocker();
  }

  clearWindow(webContentsId: number): void {
    this.windowRunning.delete(webContentsId);
    this.syncBlocker();
  }

  isBlockerActive(): boolean {
    return this.blockerId !== null
      && this.powerSaveBlocker.isStarted(this.blockerId);
  }

  private shouldBlockSleep(): boolean {
    switch (this.policy) {
      case 'off':
        return false;
      case 'whileAppRunning':
        return this.appSessionActive;
      case 'whileAgentRunning':
        for (const running of this.windowRunning.values()) {
          if (running) {
            return true;
          }
        }
        return false;
      default:
        return false;
    }
  }

  private syncBlocker(): void {
    const shouldBlock = this.shouldBlockSleep();
    if (shouldBlock) {
      if (this.blockerId === null || !this.powerSaveBlocker.isStarted(this.blockerId)) {
        this.blockerId = this.powerSaveBlocker.start('prevent-app-suspension');
      }
      return;
    }

    if (this.blockerId !== null && this.powerSaveBlocker.isStarted(this.blockerId)) {
      this.powerSaveBlocker.stop(this.blockerId);
    }
    this.blockerId = null;
  }
}
