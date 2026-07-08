import type { TFunction } from 'i18next';
import type { LoginRequiredReason } from '../../store/loginRequiredStore';

export function loginRequiredReasonKey(reason: LoginRequiredReason): string {
  switch (reason) {
    case 'command':
      return 'dialog:auth.reasonCommand';
    case 'plan':
      return 'dialog:auth.reasonPlan';
    case 'thread-control':
      return 'dialog:auth.reasonThreadControl';
    case 'compact':
      return 'dialog:auth.reasonCompact';
    case 'resume':
      return 'dialog:auth.reasonResume';
    case 'chat':
    default:
      return 'dialog:auth.reasonChat';
  }
}

export function translateLoginRequiredReason(reason: LoginRequiredReason, t: TFunction): string {
  return t(loginRequiredReasonKey(reason));
}
