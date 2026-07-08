import { create } from 'zustand';

export type LoginRequiredReason =
  | 'chat'
  | 'command'
  | 'plan'
  | 'thread-control'
  | 'compact'
  | 'resume';

type LoginRequiredState = {
  open: boolean;
  reason: LoginRequiredReason;
  show: (reason?: LoginRequiredReason) => void;
  hide: () => void;
};

export const useLoginRequiredStore = create<LoginRequiredState>((set) => ({
  open: false,
  reason: 'chat',
  show: (reason = 'chat') => set({ open: true, reason }),
  hide: () => set({ open: false }),
}));

export function showLoginRequiredDialog(reason: LoginRequiredReason = 'chat'): void {
  useLoginRequiredStore.getState().show(reason);
}
