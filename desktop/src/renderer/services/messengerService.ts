import type { WSConnection } from './wsConnection';
import type {
  MessengerChannelSummary,
  MessengerRecordAnswer,
  MessengerRecord,
} from '../store/messengerStore';

export type MessengerListResult = {
  channels?: MessengerChannelSummary[];
  messages?: MessengerRecord[];
};

export type MessengerChannelResult = {
  channelID?: string;
  threadID?: string;
  agentID?: string;
  messages?: MessengerRecord[];
};

export type MessengerReplyInput = {
  channelID: string;
  replyToMessageID?: string;
  text?: string;
  actionID?: string;
  answers?: MessengerRecordAnswer[];
  modelKey?: string;
  thinkingLevel?: string;
  contextWindow?: number;
  serviceTier?: string;
};

export type MessengerReplyResult = {
  record: MessengerRecord;
  resolved?: MessengerRecord | null;
};

export type MessengerArchiveInput = {
  channelID?: string;
  messageID?: string;
  agentID?: string;
  pendingRequestsOnly?: boolean;
};

export class MessengerService {
  constructor(private connection: WSConnection) {}

  async list(params: { threadID?: string; agentID?: string; limit?: number } = {}): Promise<MessengerListResult> {
    return this.connection.request<MessengerListResult>('messenger/list', params);
  }

  async channel(params: {
    channelID?: string;
    threadID?: string;
    agentID?: string;
    pendingOnly?: boolean;
    limit?: number;
  }): Promise<MessengerChannelResult> {
    return this.connection.request<MessengerChannelResult>('messenger/channel', params);
  }

  async reply(input: MessengerReplyInput): Promise<MessengerReplyResult> {
    return this.connection.request<MessengerReplyResult>('messenger/reply', input);
  }

  async markRead(input: {
    channelID?: string;
    threadID?: string;
    agentID?: string;
    messageIDs?: string[];
  }): Promise<{ acked: number }> {
    return this.connection.request<{ acked: number }>('messenger/markRead', input);
  }

  async archive(input: MessengerArchiveInput): Promise<{ archived: number }> {
    return this.connection.request<{ archived: number }>('messenger/archive', input);
  }
}

export function createMessengerService(connection: WSConnection): MessengerService {
  return new MessengerService(connection);
}
