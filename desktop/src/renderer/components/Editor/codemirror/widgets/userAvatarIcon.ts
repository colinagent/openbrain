/** Matches `UserIcon` in Icons/index.tsx for DOM widgets (CodeMirror). */
export const USER_AVATAR_ICON_SVG =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';

export function createUserAvatarIconElement(): HTMLSpanElement {
  const icon = document.createElement('span');
  icon.className = 'cm-md-chat-avatar-icon';
  icon.setAttribute('aria-hidden', 'true');
  icon.innerHTML = USER_AVATAR_ICON_SVG;
  return icon;
}
