import type { Hex } from "viem";
import { rpcRequest } from "./transport";
import type { AppConfig } from "./profile";

export interface BlockUpdate {
  blockNumber: bigint;
  timestamp: number;
}

export type BlockListener = (block: BlockUpdate) => void;
export type ConnectionStateListener = (state: ConnectionState) => void;

export type ConnectionState = "connected" | "connecting" | "disconnected" | "polling";

interface TendermintNewBlockHeaderEvent {
  header?: {
    height?: string;
    time?: string;
  };
}

interface TendermintSubscribeResult {
  query?: string;
}

interface TendermintRpcMessage {
  id?: string | number;
  jsonrpc?: string;
  result?: TendermintSubscribeResult | Record<string, unknown>;
  error?: { code?: number; message?: string; data?: string };
  data?: {
    type?: string;
    value?: TendermintNewBlockHeaderEvent;
  };
}

const WEBSOCKET_RECONNECT_INITIAL_MS = 1_000;
const WEBSOCKET_RECONNECT_MAX_MS = 30_000;
const WEBSOCKET_MAX_FAILURES = 3;
const POLLING_FALLBACK_INTERVAL_MS = 90_000;

/**
 * Manages block subscriptions via Tendermint WebSocket with automatic reconnection
 * and polling fallback. Subscribes to NewBlockHeader events for real-time updates.
 */
export class BlockSubscriptionManager {
  private ws: WebSocket | null = null;
  private blockListeners = new Set<BlockListener>();
  private stateListeners = new Set<ConnectionStateListener>();
  private reconnectTimer: number | null = null;
  private pollingTimer: number | null = null;
  private endpoint: string;
  private cfg: AppConfig;
  private currentDelay = WEBSOCKET_RECONNECT_INITIAL_MS;
  private failureCount = 0;
  private connectionState: ConnectionState = "disconnected";
  private isPollingFallback = false;

  constructor(endpoint: string, cfg: AppConfig) {
    this.endpoint = endpoint;
    this.cfg = cfg;
  }

  /**
   * Subscribe to block updates. Returns unsubscribe function.
   * First subscription triggers connection, last unsubscribe disconnects.
   */
  subscribeToBlocks(listener: BlockListener): () => void {
    this.blockListeners.add(listener);
    if (this.blockListeners.size === 1) {
      this.connect();
    }
    return () => {
      this.blockListeners.delete(listener);
      if (this.blockListeners.size === 0) {
        this.disconnect();
      }
    };
  }

  /**
   * Subscribe to connection state changes.
   */
  subscribeToState(listener: ConnectionStateListener): () => void {
    this.stateListeners.add(listener);
    listener(this.connectionState);
    return () => this.stateListeners.delete(listener);
  }

  private setConnectionState(state: ConnectionState): void {
    if (this.connectionState !== state) {
      this.connectionState = state;
      this.stateListeners.forEach((listener) => listener(state));
    }
  }

  private notifyBlockUpdate(block: BlockUpdate): void {
    this.blockListeners.forEach((listener) => listener(block));
  }

  private connect(): void {
    if (this.isPollingFallback) {
      this.startPollingFallback();
      return;
    }

    if (this.ws && this.ws.readyState !== WebSocket.CLOSED) {
      return;
    }

    this.setConnectionState("connecting");
    this.clearTimers();

    try {
      this.ws = new WebSocket(this.endpoint);

      this.ws.onopen = () => {
        this.failureCount = 0;
        this.currentDelay = WEBSOCKET_RECONNECT_INITIAL_MS;
        this.setConnectionState("connected");
        this.subscribeToNewBlockHeaders();
      };

      this.ws.onmessage = (event) => {
        this.handleMessage(event.data);
      };

      this.ws.onerror = () => {
        // Error details are not available in browser WebSocket
        // Will be handled by onclose
      };

      this.ws.onclose = () => {
        this.ws = null;
        this.failureCount++;

        if (this.blockListeners.size === 0) {
          this.setConnectionState("disconnected");
          return;
        }

        if (this.failureCount >= WEBSOCKET_MAX_FAILURES) {
          this.isPollingFallback = true;
          this.startPollingFallback();
        } else {
          this.scheduleReconnect();
        }
      };
    } catch {
      this.failureCount++;
      if (this.failureCount >= WEBSOCKET_MAX_FAILURES) {
        this.isPollingFallback = true;
        this.startPollingFallback();
      } else {
        this.scheduleReconnect();
      }
    }
  }

  private subscribeToNewBlockHeaders(): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }

    const subscribeMessage = {
      jsonrpc: "2.0",
      method: "subscribe",
      id: "1",
      params: {
        query: "tm.event='NewBlockHeader'",
      },
    };

    this.ws.send(JSON.stringify(subscribeMessage));
  }

  private handleMessage(data: string): void {
    try {
      const message = JSON.parse(data) as TendermintRpcMessage;

      // Handle subscription confirmation
      if (message.id === "1" && message.result) {
        return;
      }

      // Handle subscription error
      if (message.id === "1" && message.error) {
        this.failureCount++;
        if (this.failureCount >= WEBSOCKET_MAX_FAILURES) {
          this.isPollingFallback = true;
          this.disconnect();
          this.startPollingFallback();
        }
        return;
      }

      // Handle NewBlockHeader event
      if (message.data?.type === "tendermint/event/NewBlockHeader") {
        const header = message.data.value?.header;
        if (header?.height && header?.time) {
          const blockNumber = BigInt(header.height);
          const timestamp = Math.floor(new Date(header.time).getTime() / 1000);
          this.notifyBlockUpdate({ blockNumber, timestamp });
        }
      }
    } catch {
      // Ignore malformed messages
    }
  }

  private scheduleReconnect(): void {
    this.setConnectionState("connecting");
    this.clearTimers();

    this.reconnectTimer = window.setTimeout(() => {
      this.connect();
    }, this.currentDelay);

    // Exponential backoff
    this.currentDelay = Math.min(this.currentDelay * 2, WEBSOCKET_RECONNECT_MAX_MS);
  }

  private async startPollingFallback(): Promise<void> {
    this.setConnectionState("polling");
    this.clearTimers();

    const poll = async () => {
      try {
        const blockNumberHex = await rpcRequest<Hex>(this.cfg, "eth_blockNumber");
        const blockNumber = BigInt(blockNumberHex);
        const timestamp = Math.floor(Date.now() / 1000);
        this.notifyBlockUpdate({ blockNumber, timestamp });
      } catch {
        // Ignore polling errors, will retry on next interval
      }
    };

    // Immediate first poll
    await poll();

    // Schedule recurring polls
    this.pollingTimer = window.setInterval(() => void poll(), POLLING_FALLBACK_INTERVAL_MS);
  }

  private clearTimers(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.pollingTimer !== null) {
      clearInterval(this.pollingTimer);
      this.pollingTimer = null;
    }
  }

  private disconnect(): void {
    this.clearTimers();

    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onerror = null;
      this.ws.onclose = null;

      if (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING) {
        this.ws.close();
      }
      this.ws = null;
    }

    this.setConnectionState("disconnected");
  }

  /**
   * Get current connection state.
   */
  getState(): ConnectionState {
    return this.connectionState;
  }

  /**
   * Force a reconnection attempt (clears failure count).
   */
  reconnect(): void {
    this.failureCount = 0;
    this.isPollingFallback = false;
    this.disconnect();
    if (this.blockListeners.size > 0) {
      this.connect();
    }
  }
}
