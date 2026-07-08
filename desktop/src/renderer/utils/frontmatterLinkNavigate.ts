import { useAppStore } from '../store/appStore';
import { useTabManagerStore } from '../store/tabManagerStore';
import { useToastStore } from '../store/toastStore';
import { getThreadMeta } from '../services/threadService';
import { resolveAgentDefinitionPath } from './agentDefinitionPath';
import { parseAgentLinkTarget } from '../components/Editor/codemirror/utils/agentMention';
import { parseThreadLinkTarget } from './threadLink';

async function openAgentLink(agentID: string): Promise<void> {
  try {
    const store = useAppStore.getState();
    const indexedBefore = store.resolveAgentByID(agentID);
    const record = await store.ensureAgentRecord(agentID);
    const indexedAfter = useAppStore.getState().resolveAgentByID(agentID) || indexedBefore;
    const agentPath = resolveAgentDefinitionPath(record, indexedAfter);
    if (!agentPath) {
      throw new Error(`Agent not found: ${agentID}`);
    }
    await useAppStore.getState().openFile(agentPath);
  } catch (error) {
    useToastStore.getState().pushToast(
      error instanceof Error ? error.message : 'Failed to open agent',
    );
  }
}

async function openThreadLink(threadID: string): Promise<void> {
  const workspaceTabId = useTabManagerStore.getState().activeTabId;
  try {
    const meta = await getThreadMeta({ threadID }, workspaceTabId);
    const threadFilePath = (meta.threadFilePath || '').trim();
    if (!threadFilePath) {
      throw new Error('Thread JSONL path is missing.');
    }
    await useAppStore.getStoreByTabId(workspaceTabId).getState().openFile(threadFilePath);
  } catch (error) {
    useToastStore.getState().pushToast(
      error instanceof Error ? error.message : 'Failed to open thread file',
    );
  }
}

export async function navigateFrontmatterLink(target: string): Promise<void> {
  const trimmed = target.trim();
  if (!trimmed) {
    return;
  }
  const agentID = parseAgentLinkTarget(trimmed);
  if (agentID) {
    await openAgentLink(agentID);
    return;
  }
  const threadLink = parseThreadLinkTarget(trimmed);
  if (threadLink) {
    await openThreadLink(threadLink.threadID);
  }
}
