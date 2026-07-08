// WebSocket connection manager with JSON-RPC support
// VS Code / code-server level connection stability

export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

export interface FileChange {
  type: 'created' | 'changed' | 'deleted';
  path: string;
}

export interface CommandStateNotification {
  commandID: string;
  filePath: string;
  state: 'started' | 'finished' | 'failed' | 'cancelled' | 'output_limit_exceeded' | 'timeout';
  exitCode?: number | null;
  error?: string;
}

export interface MessengerMessageNotification {
  id: string;
  channelID: string;
  threadID: string;
  agentID: string;
  sender: 'user' | 'agent' | 'system';
  kind: 'message' | 'request' | 'status';
  status: 'open' | 'resolved' | 'archived';
  title?: string;
  body: string;
  actions?: Array<{ id: string; label: string; tone?: 'primary' | 'danger' }>;
  questions?: Array<{
    id: string;
    question: string;
    options?: Array<{ id: string; label: string }>;
  }>;
  replyToMessageID?: string;
  actionID?: string;
  answers?: Array<{ questionID: string; optionID?: string; label?: string; other?: boolean; text?: string }>;
  createdAt: string;
  updatedAt: string;
  meta?: Record<string, unknown>;
}

interface RPCRequest {
  jsonrpc: '2.0';
  id: number;
  method: string;
  params?: unknown;
}

interface RPCResponse {
  jsonrpc: '2.0';
  id: number;
  result?: unknown;
  error?: {
    code: number;
    message: string;
    data?: unknown;
  };
}

interface RPCNotification {
  jsonrpc: '2.0';
  method: string;
  params?: unknown;
}

export interface ConnectionCallbacks {
  onStateChange?: (state: ConnectionState) => void;
  onConnect?: () => void;
  onDisconnect?: () => void;
  onFileChange?: (changes: FileChange[]) => void;
  onCommandState?: (event: CommandStateNotification) => void;
  onMessengerMessage?: (message: MessengerMessageNotification) => void;
}

type PendingRequest = {
  resolve: (result: unknown) => void;
  reject: (error: Error) => void;
  timeout: ReturnType<typeof setTimeout>;
};

export class WSConnection {
  private ws: WebSocket | null = null;
  private url: string = '';
  private callbacks: ConnectionCallbacks = {};
  private requestId: number = 0;
  private pendingRequests: Map<number, PendingRequest> = new Map();
  private suspended = false;
  private disposed = false;

  // Connection state
  private _state: ConnectionState = 'disconnected';

  // Reconnection parameters (aligned with VS Code / code-server)
  private readonly minReconnectDelay = 1000;       // 1s
  private readonly maxReconnectDelay = 30000;      // 30s
  private readonly reconnectDelayGrowFactor = 1.5; // exponential growth
  private readonly jitterFactor = 0.3;             // 0-30% jitter
  private reconnectDelay = this.minReconnectDelay;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  // Network status
  private isOnline = typeof navigator !== 'undefined' ? navigator.onLine : true;

  // Request timeout
  private readonly requestTimeout = 30000; // 30s
  private readonly connectTimeoutMs = 8000; // 8s
  private connectTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
  private connectAttemptId = 0;

  private clearReconnectTimer() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private clearConnectTimeout() {
    if (this.connectTimeoutTimer) {
      clearTimeout(this.connectTimeoutTimer);
      this.connectTimeoutTimer = null;
    }
  }

  private hasHealthyConnectingAttempt(): boolean {
    return this._state === 'connecting'
      && this.ws?.readyState === WebSocket.CONNECTING
      && this.connectTimeoutTimer !== null;
  }

  private discardSocket(close = true) {
    if (!this.ws) {
      return;
    }
    const socket = this.ws;
    socket.onopen = null;
    socket.onclose = null;
    socket.onerror = null;
    socket.onmessage = null;
    if (close && socket.readyState !== WebSocket.CLOSED) {
      try {
        socket.close();
      } catch {
        // Ignore close failures while force-resetting the socket.
      }
    }
    this.ws = null;
  }

  private resetCurrentConnection(reason: string, options?: { notifyDisconnect?: boolean }) {
    const hadSocket = Boolean(this.ws);
    const shouldNotifyDisconnect = options?.notifyDisconnect === true
      && (hadSocket || this._state !== 'disconnected');
    this.clearConnectTimeout();
    this.discardSocket(true);
    this.rejectAllPendingRequests(reason);
    if (shouldNotifyDisconnect) {
      this.callbacks.onDisconnect?.();
    }
  }

  private startConnectTimeout(attemptId: number, socket: WebSocket) {
    this.clearConnectTimeout();
    this.connectTimeoutTimer = setTimeout(() => {
      if (this.disposed || this.suspended) {
        return;
      }
      if (attemptId !== this.connectAttemptId || this.ws !== socket) {
        return;
      }
      console.warn(`[WS] Connect timed out after ${this.connectTimeoutMs}ms`);
      this.resetCurrentConnection('Connection timed out', { notifyDisconnect: true });
      this.setState('reconnecting');
      this.scheduleReconnect();
    }, this.connectTimeoutMs);
  }

  private readonly handleOnline = () => {
    if (this.disposed) {
      return;
    }
    console.log('[WS] Network online');
    this.isOnline = true;

    if (this._state !== 'connected') {
      this.reconnectDelay = this.minReconnectDelay;
      this.clearReconnectTimer();
      this.forceReconnect('network-online');
    }
  };

  private readonly handleOffline = () => {
    if (this.disposed) {
      return;
    }
    console.log('[WS] Network offline');
    this.isOnline = false;
  };

  private readonly handleVisibilityChange = () => {
    if (this.disposed || typeof document === 'undefined' || document.visibilityState !== 'visible') {
      return;
    }

    console.log('[WS] Page visible');

    if (!this.suspended && this._state !== 'connected') {
      console.log('[WS] Not connected, triggering reconnect');
      this.reconnectDelay = this.minReconnectDelay;
      this.clearReconnectTimer();
      this.forceReconnect('page-visible');
    } else if (!this.suspended && this.ws?.readyState !== WebSocket.OPEN) {
      console.log('[WS] WebSocket not open, triggering reconnect');
      this.reconnectDelay = this.minReconnectDelay;
      this.clearReconnectTimer();
      this.forceReconnect('page-visible');
    }
  };

  constructor() {
    this.setupNetworkListeners();
    this.setupVisibilityListener();
  }

  get state(): ConnectionState {
    return this._state;
  }

  private setState(newState: ConnectionState) {
    if (this._state !== newState) {
      this._state = newState;
      this.callbacks.onStateChange?.(newState);
    }
  }

  connect(url: string, callbacks: ConnectionCallbacks) {
    if (this.disposed) {
      return;
    }

    // Update callbacks
    this.callbacks = callbacks;

    const isSameUrl = this.url === url;
    if (this.url && !isSameUrl) {
      this.disconnect();
    }

    if (isSameUrl && this._state === 'connected' && this.ws?.readyState === WebSocket.OPEN) {
      // Notify current state to new callbacks
      callbacks.onStateChange?.(this._state);
      callbacks.onConnect?.();
      return;
    }

    if (isSameUrl && this.hasHealthyConnectingAttempt()) {
      callbacks.onStateChange?.(this._state);
      return;
    }

    this.url = url;
    if (this.suspended) {
      this.setState('disconnected');
      return;
    }

    if (isSameUrl && this._state === 'connecting') {
      this.forceReconnect('stale-connecting-connect');
      return;
    }

    this.setState('connecting');
    this.doConnect();
  }

  forceReconnect(reason?: string) {
    if (this.disposed) {
      return;
    }
    if (!this.url) {
      this.setState('disconnected');
      return;
    }

    if (reason) {
      console.log(`[WS] Force reconnect (${reason})`);
    } else {
      console.log('[WS] Force reconnect');
    }

    this.clearReconnectTimer();
    this.reconnectDelay = this.minReconnectDelay;
    this.resetCurrentConnection(
      reason ? `Connection reset (${reason})` : 'Connection reset',
      { notifyDisconnect: true },
    );

    if (this.suspended) {
      this.setState('disconnected');
      return;
    }

    this.setState('reconnecting');
    this.doConnect({ force: true });
  }

  private doConnect(options?: { force?: boolean }) {
    const force = options?.force === true;
    if (this.suspended || this.disposed) {
      return;
    }
    // Don't connect if already connected or connecting
    if (!force && (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING)) {
      return;
    }

    // Don't try to connect if offline
    if (!this.isOnline) {
      console.log('[WS] Offline, waiting for network...');
      return;
    }

    // Clean up existing connection if any
    if (this.ws) {
      this.clearConnectTimeout();
      this.discardSocket(true);
    }

    try {
      console.log(`[WS] Connecting to ${this.url}...`);
      const socket = new WebSocket(this.url);
      this.ws = socket;
      const attemptId = ++this.connectAttemptId;
      this.startConnectTimeout(attemptId, socket);

      socket.onopen = () => {
        if (this.ws !== socket) {
          return;
        }
        this.clearConnectTimeout();
        console.log('[WS] Connected');
        this.setState('connected');
        // Reset reconnect delay on successful connection
        this.reconnectDelay = this.minReconnectDelay;
        this.callbacks.onConnect?.();
      };

      socket.onclose = (event) => {
        if (this.ws === socket) {
          this.clearConnectTimeout();
          this.ws = null;
        }
        console.log(`[WS] Disconnected (code: ${event.code}, reason: ${event.reason || 'none'})`);
        
        // Reject all pending requests immediately
        this.rejectAllPendingRequests('Connection closed');
        
        // Notify disconnect
        this.callbacks.onDisconnect?.();

        if (this.suspended || this.disposed) {
          this.setState('disconnected');
          return;
        }
        
        // Schedule reconnection
        this.setState('reconnecting');
        this.scheduleReconnect();
      };

      socket.onerror = (error) => {
        console.error('[WS] Error:', error);
        // onclose will be called after onerror, so we don't need to handle reconnection here
      };

      socket.onmessage = (event) => {
        this.handleMessage(event.data);
      };
    } catch (error) {
      this.clearConnectTimeout();
      console.error('[WS] Failed to connect:', error);
      this.setState('reconnecting');
      this.scheduleReconnect();
    }
  }

  private scheduleReconnect() {
    if (this.disposed) {
      return;
    }

    // Clear any existing timer
    this.clearReconnectTimer();

    // Don't reconnect if offline - the online event will trigger reconnection
    if (!this.isOnline) {
      console.log('[WS] Offline, will reconnect when online');
      return;
    }

    // Add jitter to avoid thundering herd
    const jitter = Math.random() * this.jitterFactor * this.reconnectDelay;
    const delay = Math.min(this.reconnectDelay + jitter, this.maxReconnectDelay);

    console.log(`[WS] Reconnecting in ${Math.round(delay)}ms...`);

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.doConnect();
    }, delay);

    // Exponential backoff for next attempt
    this.reconnectDelay = Math.min(
      this.reconnectDelay * this.reconnectDelayGrowFactor,
      this.maxReconnectDelay
    );
  }

  private setupNetworkListeners() {
    if (typeof window === 'undefined') return;
    window.addEventListener('online', this.handleOnline);
    window.addEventListener('offline', this.handleOffline);
  }

  private setupVisibilityListener() {
    if (typeof document === 'undefined') return;
    document.addEventListener('visibilitychange', this.handleVisibilityChange);
  }

  private removeGlobalListeners() {
    if (typeof window !== 'undefined') {
      window.removeEventListener('online', this.handleOnline);
      window.removeEventListener('offline', this.handleOffline);
    }
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.handleVisibilityChange);
    }
  }

  suspend() {
    if (this.suspended || this.disposed) {
      return;
    }
    this.suspended = true;
    this.disconnect();
  }

  resume() {
    if (!this.suspended || this.disposed) {
      return;
    }
    this.suspended = false;
    if (this.url) {
      this.setState('reconnecting');
      this.doConnect({ force: true });
    }
  }

  private rejectAllPendingRequests(reason: string) {
    for (const [id, pending] of this.pendingRequests) {
      clearTimeout(pending.timeout);
      pending.reject(new Error(reason));
    }
    this.pendingRequests.clear();
  }

  disconnect() {
    this.clearReconnectTimer();
    this.clearConnectTimeout();
    this.discardSocket(true);

    // Reject all pending requests
    this.rejectAllPendingRequests('Connection closed');

    this.setState('disconnected');
  }

  dispose() {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    this.callbacks = {};
    this.url = '';
    this.disconnect();
    this.removeGlobalListeners();
  }

  isConnected(): boolean {
    return this._state === 'connected' && this.ws?.readyState === WebSocket.OPEN;
  }

  private handleMessage(data: string) {
    try {
      const message = JSON.parse(data);

      // Check if it's a response to a request
      if ('id' in message && message.id !== null) {
        const pending = this.pendingRequests.get(message.id);
        if (pending) {
          clearTimeout(pending.timeout);
          this.pendingRequests.delete(message.id);

          if (message.error) {
            pending.reject(new Error(message.error.message));
          } else {
            pending.resolve(message.result);
          }
        }
        return;
      }

      // It's a notification
      this.handleNotification(message as RPCNotification);
    } catch (error) {
      console.error('[WS] Failed to parse message:', error);
    }
  }

  private handleNotification(notification: RPCNotification) {
    switch (notification.method) {
      case 'fs/fileChange':
        const event = notification.params as { watchId: string; changes: FileChange[] };
        this.callbacks.onFileChange?.(event.changes);
        break;
      case 'command/state':
        this.callbacks.onCommandState?.(notification.params as CommandStateNotification);
        break;
      case 'messenger/message':
        this.callbacks.onMessengerMessage?.(notification.params as MessengerMessageNotification);
        break;
      default:
        console.log('[WS] Unknown notification:', notification.method);
    }
  }

  async request<T>(method: string, params?: unknown): Promise<T> {
    if (this.disposed) {
      throw new Error('Connection disposed');
    }
    if (!this.isConnected()) {
      throw new Error('Not connected');
    }

    const id = ++this.requestId;
    const request: RPCRequest = {
      jsonrpc: '2.0',
      id,
      method,
      params,
    };

    return new Promise<T>((resolve, reject) => {
      // Set timeout for request
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new Error('Request timeout'));
      }, this.requestTimeout);

      this.pendingRequests.set(id, {
        resolve: resolve as (result: unknown) => void,
        reject,
        timeout,
      });

      this.ws!.send(JSON.stringify(request));
    });
  }
}
