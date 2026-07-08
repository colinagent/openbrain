import { WidgetType } from '@codemirror/view';
import { useAppStore } from '../../../../store/appStore';
import { buildAgentLinkTarget } from '../utils/agentMention';

type AgentMentionWidgetOptions = {
  agentID: string;
  className?: string;
};

type AgentNameSource = {
  name?: unknown;
  meta?: unknown;
};

function readAgentName(value: AgentNameSource | null | undefined): string {
  const direct = typeof value?.name === 'string' ? value.name.trim() : '';
  if (direct) {
    return direct;
  }
  const meta = value?.meta && typeof value.meta === 'object'
    ? value.meta as Record<string, unknown>
    : null;
  const fromMeta = typeof meta?.name === 'string' ? meta.name.trim() : '';
  return fromMeta;
}

function renderLabel(agentID: string, name: string | null | undefined): string {
  return (name || '').trim() || `@${agentID}`;
}

function renderTitle(agentID: string, name: string | null | undefined): string {
  const resolvedName = (name || '').trim();
  return resolvedName ? `${resolvedName}\nAgent ID: ${agentID}` : `Agent ID: ${agentID}`;
}

export class AgentMentionWidget extends WidgetType {
  private readonly agentID: string;
  private readonly className: string;

  constructor(options: AgentMentionWidgetOptions) {
    super();
    this.agentID = options.agentID;
    this.className = (options.className || '').trim();
  }

  eq(other: AgentMentionWidget): boolean {
    return other.agentID === this.agentID && other.className === this.className;
  }

  toDOM(): HTMLElement {
    const element = document.createElement('span');
    const target = buildAgentLinkTarget(this.agentID) || '';
    const indexed = useAppStore.getState().resolveAgentByID(this.agentID);
    const initialName = readAgentName(indexed);

    element.className = ['cm-md-link', 'cm-md-agent-mention', this.className].filter(Boolean).join(' ');
    element.dataset.mdLink = target;
    element.dataset.agentId = this.agentID;
    element.textContent = renderLabel(this.agentID, initialName);
    element.title = renderTitle(this.agentID, initialName);

    void useAppStore.getState().ensureAgentRecord(this.agentID).then((record) => {
      if (!element.isConnected) {
        return;
      }
      const name = readAgentName(record) || readAgentName(useAppStore.getState().resolveAgentByID(this.agentID));
      element.textContent = renderLabel(this.agentID, name);
      element.title = renderTitle(this.agentID, name);
    });

    return element;
  }

  ignoreEvent(): boolean {
    return false;
  }
}
