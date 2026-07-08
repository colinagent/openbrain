import { WidgetType } from '@codemirror/view';
import { buildInitials, initialsBackgroundColor } from '../../../avatarInitials';
import { OP_SG_CAPSULE, OP_SG_CAPSULE_ON_EDITOR } from '../../../staticGlassCapsule';
import { resolveUserAvatarSrc } from '../../../TitlebarUserAvatar';
import { resolveLooseResourceUrl } from '../../../../services/resourceService';
import { useAppStore } from '../../../../store/appStore';
import { useAuthStore } from '../../../../store/authStore';
import { createUserAvatarIconElement } from './userAvatarIcon';

export type ChatHeaderRole = 'me' | 'agent';

type ChatHeaderWidgetOptions = {
  role: ChatHeaderRole;
  displayName: string;
  agentID?: string | null;
  msgId?: string | null;
  timestamp?: string | null;
  isCurrentUser?: boolean;
};

function applyFallbackStyle(el: HTMLElement, name: string): void {
  const displayName = name || 'User';
  el.textContent = buildInitials(displayName);
  el.style.backgroundColor = initialsBackgroundColor(displayName);
}

function resolveAvatarUrl(url: string | null | undefined): string {
  const trimmed = (url || '').trim();
  if (!trimmed) return '';
  return trimmed;
}

function setAvatar(img: HTMLImageElement, fallback: HTMLElement, url: string | null | undefined) {
  const next = resolveAvatarUrl(url);
  if (!next) {
    img.removeAttribute('src');
    img.style.display = 'none';
    fallback.style.display = '';
    return;
  }

  fallback.style.display = '';
  img.style.display = 'none';
  void resolveLooseResourceUrl(next)
    .then((resolved) => {
      if (!resolved) {
        return;
      }
      img.src = resolved;
      img.style.display = 'block';
      fallback.style.display = 'none';
      img.onerror = () => {
        img.style.display = 'none';
        fallback.style.display = '';
        img.onerror = null;
      };
    })
    .catch(() => {
      img.style.display = 'none';
      fallback.style.display = '';
    });
}

function showGuestUserAvatar(
  icon: HTMLElement,
  fallback: HTMLElement,
  img: HTMLImageElement,
): void {
  icon.style.display = 'grid';
  fallback.style.display = 'none';
  img.style.display = 'none';
  img.removeAttribute('src');
}

function applyCurrentUserAvatar(
  icon: HTMLElement,
  fallback: HTMLElement,
  img: HTMLImageElement,
  displayName: string,
  avatarSrc: string | null,
): void {
  icon.style.display = 'none';
  applyFallbackStyle(fallback, displayName);
  setAvatar(img, fallback, avatarSrc);
}

type AgentMetaLike = {
  avatar?: string;
  name?: unknown;
  description?: unknown;
  model?: unknown;
  run?: unknown;
  opcodes?: unknown;
};

function asTrimmedString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function parseOpCodeLabel(value: unknown): string {
  if (typeof value === 'string') {
    return value.trim();
  }
  if (!value || typeof value !== 'object') {
    return '';
  }
  const rec = value as Record<string, unknown>;
  return asTrimmedString(rec.opcode) || asTrimmedString(rec.code) || asTrimmedString(rec.name) || '';
}

function parseOpCodes(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const uniq = new Set<string>();
  for (const item of value) {
    const label = parseOpCodeLabel(item);
    if (label) {
      uniq.add(label);
    }
  }
  return Array.from(uniq);
}

function parseDaemon(value: unknown): boolean {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const run = value as Record<string, unknown>;
  return run.daemon === true;
}

function buildUserTooltipText(params: {
  displayName: string;
  username?: string | null;
  uid?: string | null;
  email?: string | null;
}): string {
  const lines: string[] = [];
  const name = (params.username || '').trim() || (params.displayName || '').trim() || 'Me';
  const uid = (params.uid || '').trim();
  const email = (params.email || '').trim();
  lines.push(`Name: ${name}`);
  if (uid) {
    lines.push(`UID: ${uid}`);
  }
  if (email) {
    lines.push(`Email: ${email}`);
  }
  return lines.join('\n');
}

function buildAgentTooltipText(params: {
  displayName: string;
  agentID?: string | null;
  indexedName?: string | null;
  uri?: string | null;
  meta?: AgentMetaLike | null;
}): string {
  const lines: string[] = [];
  const name = asTrimmedString(params.meta?.name) || (params.indexedName || '').trim() || (params.displayName || '').trim();
  const description = asTrimmedString(params.meta?.description);
  const model = asTrimmedString(params.meta?.model);
  const daemon = parseDaemon(params.meta?.run);
  const opcodes = parseOpCodes(params.meta?.opcodes);
  const agentID = (params.agentID || '').trim();
  const uri = (params.uri || '').trim();
  if (name) lines.push(`Name: ${name}`);
  if (description) lines.push(`Description: ${description}`);
  if (model) lines.push(`Model: ${model}`);
  if (daemon) lines.push('Daemon: true');
  if (opcodes.length > 0) lines.push(`OpCodes: ${opcodes.join(', ')}`);
  if (agentID) lines.push(`Agent ID: ${agentID}`);
  if (uri) lines.push(`URI: ${uri}`);
  if (lines.length === 0) {
    lines.push('Agent');
  }
  return lines.join('\n');
}

export class ChatHeaderWidget extends WidgetType {
  private role: ChatHeaderRole;
  private displayName: string;
  private agentID: string | null;
  private msgId: string | null;
  private timestamp: string | null;
  private isCurrentUser: boolean;

  constructor(options: ChatHeaderWidgetOptions) {
    super();
    this.role = options.role;
    this.displayName = options.displayName;
    this.agentID = options.agentID ?? null;
    this.msgId = options.msgId ?? null;
    this.timestamp = options.timestamp ?? null;
    this.isCurrentUser = options.isCurrentUser ?? false;
  }

  eq(other: ChatHeaderWidget): boolean {
    return (
      other.role === this.role &&
      other.displayName === this.displayName &&
      other.agentID === this.agentID &&
      other.msgId === this.msgId &&
      other.timestamp === this.timestamp &&
      other.isCurrentUser === this.isCurrentUser
    );
  }

  ignoreEvent(): boolean {
    return true;
  }

  toDOM(): HTMLElement {
    const wrap = document.createElement('span');
    wrap.className = `cm-md-chat-header cm-md-chat-inline cm-md-chat-${this.role}`;

    const avatar = document.createElement('span');
    avatar.className = ['cm-md-chat-avatar', OP_SG_CAPSULE, OP_SG_CAPSULE_ON_EDITOR].join(' ');

    const avatarIcon = createUserAvatarIconElement();
    avatarIcon.style.display = 'none';

    const avatarFallback = document.createElement('span');
    avatarFallback.className = 'cm-md-chat-avatar-fallback';
    applyFallbackStyle(avatarFallback, this.displayName || 'User');

    const img = document.createElement('img');
    img.className = 'cm-md-chat-avatar-img';
    img.alt = '';

    avatar.appendChild(avatarIcon);
    avatar.appendChild(avatarFallback);
    avatar.appendChild(img);
    wrap.appendChild(avatar);

    const name = document.createElement('span');
    name.className = 'cm-md-chat-name';
    name.textContent = this.displayName || 'User';
    wrap.appendChild(name);

    if (this.role === 'me') {
      const auth = useAuthStore.getState();
      const profile = auth.profile;
      let tooltip = buildUserTooltipText({
        displayName: this.displayName,
        username: profile?.username || null,
        uid: profile?.uid || auth.uid || null,
        email: profile?.email || auth.email || null,
      });
      if (this.msgId) tooltip += `\nID: ${this.msgId}`;
      if (this.timestamp) tooltip += `\nTS: ${this.timestamp}`;
      wrap.title = tooltip;
      if (!auth.loggedIn || !this.isCurrentUser) {
        showGuestUserAvatar(avatarIcon, avatarFallback, img);
      } else {
        applyCurrentUserAvatar(
          avatarIcon,
          avatarFallback,
          img,
          this.displayName || 'User',
          resolveUserAvatarSrc(profile),
        );
      }
    } else if (this.agentID) {
      const indexed = useAppStore.getState().resolveAgentByID(this.agentID);
      let tooltip = buildAgentTooltipText({
        displayName: this.displayName,
        agentID: this.agentID,
        indexedName: indexed?.name || null,
        uri: indexed?.uri || null,
      });
      if (this.msgId) tooltip += `\nID: ${this.msgId}`;
      if (this.timestamp) tooltip += `\nTS: ${this.timestamp}`;
      wrap.title = tooltip;
      if (indexed?.avatar) {
        setAvatar(img, avatarFallback, indexed.avatar);
      }
      void useAppStore.getState().ensureAgentRecord(this.agentID).then((record) => {
        if (!wrap.isConnected) {
          return;
        }
        const meta = (record?.meta as AgentMetaLike | undefined) || null;
        const recordUri = asTrimmedString((record as { uri?: unknown } | null)?.uri);
        setAvatar(img, avatarFallback, meta?.avatar || indexed?.avatar || null);
        let t = buildAgentTooltipText({
          displayName: this.displayName,
          agentID: this.agentID,
          indexedName: indexed?.name || null,
          uri: recordUri || indexed?.uri || null,
          meta,
        });
        if (this.msgId) t += `\nID: ${this.msgId}`;
        if (this.timestamp) t += `\nTS: ${this.timestamp}`;
        wrap.title = t;
      });
    } else {
      // No avatar URL, ensure fallback is visible
      let t = buildAgentTooltipText({ displayName: this.displayName, agentID: null });
      if (this.msgId) t += `\nID: ${this.msgId}`;
      if (this.timestamp) t += `\nTS: ${this.timestamp}`;
      wrap.title = t;
      setAvatar(img, avatarFallback, null);
    }

    return wrap;
  }
}
