export type DesktopUpdateInstallPlan = {
  awaitingWindowIds: number[];
  requestCloseWindowIds: number[];
  shouldInstallImmediately: boolean;
};

export class DesktopUpdateInstallCoordinator {
  private awaitingWindowIds = new Set<number>();

  planInstall(windowIds: Iterable<number>, pendingCloseWindowIds: Iterable<number>): DesktopUpdateInstallPlan {
    const awaiting = new Set<number>();
    const pendingClose = new Set<number>(pendingCloseWindowIds);
    const requestCloseWindowIds: number[] = [];

    for (const windowId of windowIds) {
      awaiting.add(windowId);
      if (!pendingClose.has(windowId)) {
        requestCloseWindowIds.push(windowId);
      }
    }

    this.awaitingWindowIds = awaiting;

    return {
      awaitingWindowIds: [...awaiting],
      requestCloseWindowIds,
      shouldInstallImmediately: awaiting.size === 0,
    };
  }

  markWindowClosed(windowId: number): boolean {
    if (!this.awaitingWindowIds.delete(windowId)) {
      return false;
    }
    return this.awaitingWindowIds.size === 0;
  }

  isActive(): boolean {
    return this.awaitingWindowIds.size > 0;
  }

  reset() {
    this.awaitingWindowIds.clear();
  }
}
