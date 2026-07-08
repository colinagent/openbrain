import assert from 'node:assert/strict';
import test from 'node:test';

import {
  AgentSleepInhibitorController,
  type PowerSaveBlockerAPI,
} from './agentSleepInhibitor';

class FakePowerSaveBlocker implements PowerSaveBlockerAPI {
  private nextId = 1;
  private readonly started = new Set<number>();

  start(_type: 'prevent-app-suspension' | 'prevent-display-sleep'): number {
    const id = this.nextId++;
    this.started.add(id);
    return id;
  }

  stop(id: number): boolean {
    return this.started.delete(id);
  }

  isStarted(id: number): boolean {
    return this.started.has(id);
  }

  activeCount(): number {
    return this.started.size;
  }
}

test('AgentSleepInhibitorController starts blocker when whileAgentRunning and a window is running', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAgentRunning');
  controller.setWindowRunning(1, true);

  assert.equal(controller.isBlockerActive(), true);
  assert.equal(powerSaveBlocker.activeCount(), 1);
});

test('AgentSleepInhibitorController stops blocker when running ends', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAgentRunning');
  controller.setWindowRunning(1, true);
  controller.setWindowRunning(1, false);

  assert.equal(controller.isBlockerActive(), false);
  assert.equal(powerSaveBlocker.activeCount(), 0);
});

test('AgentSleepInhibitorController ignores running windows when policy is off', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('off');
  controller.setWindowRunning(1, true);

  assert.equal(controller.isBlockerActive(), false);
  assert.equal(powerSaveBlocker.activeCount(), 0);
});

test('AgentSleepInhibitorController keeps blocker active while any window is running', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAgentRunning');
  controller.setWindowRunning(1, true);
  controller.setWindowRunning(2, true);
  controller.setWindowRunning(1, false);

  assert.equal(controller.isBlockerActive(), true);
  assert.equal(powerSaveBlocker.activeCount(), 1);
});

test('AgentSleepInhibitorController clears window state on close', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAgentRunning');
  controller.setWindowRunning(1, true);
  controller.clearWindow(1);

  assert.equal(controller.isBlockerActive(), false);
  assert.equal(powerSaveBlocker.activeCount(), 0);
});

test('AgentSleepInhibitorController releases blocker when policy changes to off', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAgentRunning');
  controller.setWindowRunning(1, true);
  controller.setPolicy('off');

  assert.equal(controller.isBlockerActive(), false);
  assert.equal(powerSaveBlocker.activeCount(), 0);
});

test('AgentSleepInhibitorController blocks while app session is active under whileAppRunning', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAppRunning');
  controller.setAppSessionActive(true);

  assert.equal(controller.isBlockerActive(), true);
  assert.equal(powerSaveBlocker.activeCount(), 1);
});

test('AgentSleepInhibitorController ignores window running under whileAppRunning', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAppRunning');
  controller.setAppSessionActive(false);
  controller.setWindowRunning(1, true);

  assert.equal(controller.isBlockerActive(), false);
});

test('AgentSleepInhibitorController releases blocker when app session ends', () => {
  const powerSaveBlocker = new FakePowerSaveBlocker();
  const controller = new AgentSleepInhibitorController({ powerSaveBlocker });

  controller.setPolicy('whileAppRunning');
  controller.setAppSessionActive(true);
  controller.setAppSessionActive(false);

  assert.equal(controller.isBlockerActive(), false);
  assert.equal(powerSaveBlocker.activeCount(), 0);
});
