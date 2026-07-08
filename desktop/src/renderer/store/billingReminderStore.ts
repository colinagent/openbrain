import { create } from 'zustand';

export type BillingReminderKind =
  | 'chat-unavailable'
  | 'bundled-token-required'
  | 'quota-exhausted';

type BillingReminderState = {
  open: boolean;
  kind: BillingReminderKind;
  show: (kind?: BillingReminderKind) => void;
  hide: () => void;
};

export const useBillingReminderStore = create<BillingReminderState>((set) => ({
  open: false,
  kind: 'chat-unavailable',
  show: (kind = 'chat-unavailable') => set({ open: true, kind }),
  hide: () => set({ open: false }),
}));
