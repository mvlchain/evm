/**
 * Cosmos SDK Event Listener for RideHail Matching
 *
 * Listens for real-time Cosmos events via WebSocket:
 * - ridehail_request_created: New ride request in pending pool
 * - driver_commit_submitted: Driver commit received
 * - ridehail_match: Successful match (Session created!)
 * - ridehail_request_expired: Request expired without match
 */

import WebSocket from "ws";

// Tendermint RPC WebSocket endpoint
const TENDERMINT_WS_URL = "ws://localhost:26657/websocket";

interface CosmosEvent {
  type: string;
  attributes: Array<{ key: string; value: string; index: boolean }>;
}

interface TendermintEvent {
  query: string;
  data: {
    type: string;
    value: {
      TxResult?: {
        height: string;
        tx: string;
        result: {
          events: CosmosEvent[];
        };
      };
      ResultBeginBlock?: {
        events: CosmosEvent[];
      };
      ResultEndBlock?: {
        events: CosmosEvent[];
      };
    };
  };
}

class RideHailEventListener {
  private ws: WebSocket | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 2000;

  constructor() {}

  /**
   * Connect to Tendermint WebSocket and subscribe to events
   */
  async connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      console.log(`🔌 Connecting to Tendermint WebSocket: ${TENDERMINT_WS_URL}`);

      this.ws = new WebSocket(TENDERMINT_WS_URL);

      this.ws.on("open", () => {
        console.log("✅ WebSocket connected!");
        this.reconnectAttempts = 0;

        // Subscribe to all transactions (includes events)
        this.subscribe("tm.event='Tx'");

        // Subscribe to BeginBlock events (where matching happens!)
        this.subscribe("tm.event='NewBlock'");

        resolve();
      });

      this.ws.on("message", (data: Buffer) => {
        this.handleMessage(data.toString());
      });

      this.ws.on("error", (error) => {
        console.error("❌ WebSocket error:", error.message);
        reject(error);
      });

      this.ws.on("close", () => {
        console.log("🔌 WebSocket disconnected");
        this.handleDisconnect();
      });
    });
  }

  /**
   * Subscribe to Tendermint events
   */
  private subscribe(query: string): void {
    if (!this.ws) return;

    const subscribeMsg = {
      jsonrpc: "2.0",
      method: "subscribe",
      id: Date.now(),
      params: {
        query: query,
      },
    };

    console.log(`📡 Subscribing to: ${query}`);
    this.ws.send(JSON.stringify(subscribeMsg));
  }

  /**
   * Handle incoming WebSocket messages
   */
  private handleMessage(data: string): void {
    try {
      const message = JSON.parse(data);

      // Handle subscription result
      if (message.result && message.result.query) {
        console.log(`✓ Subscribed to: ${message.result.query}`);
        return;
      }

      // Handle events
      if (message.result && message.result.data) {
        this.processEvent(message.result);
      }
    } catch (error: any) {
      console.error("Error parsing message:", error.message);
    }
  }

  /**
   * Process Tendermint events
   */
  private processEvent(event: TendermintEvent): void {
    const eventType = event.data.type;
    const value = event.data.value;

    // Process Tx events
    if (value.TxResult) {
      const height = value.TxResult.height;
      const events = value.TxResult.result.events;

      this.processCosmosEvents(events, height, "Tx");
    }

    // Process BeginBlock events (where matching happens!)
    if (value.ResultBeginBlock) {
      const events = value.ResultBeginBlock.events;
      // BeginBlock events don't have height in the same place, extract from block
      this.processCosmosEvents(events, "N/A", "BeginBlock");
    }

    // Process EndBlock events
    if (value.ResultEndBlock) {
      const events = value.ResultEndBlock.events;
      this.processCosmosEvents(events, "N/A", "EndBlock");
    }
  }

  /**
   * Process Cosmos SDK events
   */
  private processCosmosEvents(
    events: CosmosEvent[],
    height: string,
    source: string
  ): void {
    for (const event of events) {
      // Parse event attributes
      const attrs: Record<string, string> = {};
      for (const attr of event.attributes) {
        const key = Buffer.from(attr.key, "base64").toString();
        const value = Buffer.from(attr.value, "base64").toString();
        attrs[key] = value;
      }

      // Handle RideHail events
      switch (event.type) {
        case "ridehail_request_created":
          this.onRequestCreated(attrs, height, source);
          break;

        case "driver_commit_submitted":
          this.onDriverCommit(attrs, height, source);
          break;

        case "ridehail_match":
          this.onMatch(attrs, height, source);
          break;

        case "ridehail_request_expired":
          this.onRequestExpired(attrs, height, source);
          break;
      }
    }
  }

  /**
   * Handler: New ride request created
   */
  private onRequestCreated(
    attrs: Record<string, string>,
    height: string,
    source: string
  ): void {
    console.log("\n" + "=".repeat(60));
    console.log(`📝 NEW RIDE REQUEST [${source} @ block ${height}]`);
    console.log("=".repeat(60));
    console.log(`🎫 Request ID: ${attrs.request_id}`);
    console.log(`👤 Rider: ${attrs.rider}`);
    console.log(`📍 Cell Topic: ${attrs.cell_topic?.slice(0, 16)}...`);
    console.log(`⏰ Max ETA: ${attrs.max_eta}s`);
    console.log(`⏳ Expires at: ${new Date(parseInt(attrs.expires_at) * 1000).toISOString()}`);
    console.log(`\n💡 Request is now in PENDING POOL, waiting for drivers...`);
  }

  /**
   * Handler: Driver commit submitted
   */
  private onDriverCommit(
    attrs: Record<string, string>,
    height: string,
    source: string
  ): void {
    console.log("\n" + "=".repeat(60));
    console.log(`🚗 DRIVER COMMIT RECEIVED [${source} @ block ${height}]`);
    console.log("=".repeat(60));
    console.log(`🎫 Request ID: ${attrs.request_id}`);
    console.log(`🚗 Driver: ${attrs.driver}`);
    console.log(`⏱️  ETA: ${attrs.eta}s`);
    console.log(`\n💡 Commit is now in DRIVER POOL, matching will occur in BeginBlock...`);
  }

  /**
   * Handler: Successful match! 🎉
   */
  private onMatch(
    attrs: Record<string, string>,
    height: string,
    source: string
  ): void {
    console.log("\n" + "🎉".repeat(30));
    console.log(`✨ MATCH SUCCESSFUL! [${source} @ block ${height}]`);
    console.log("🎉".repeat(30));
    console.log(`🎫 Request ID: ${attrs.request_id}`);
    console.log(`🔗 Session ID: ${attrs.session_id}`);
    console.log(`👤 Rider: ${attrs.rider}`);
    console.log(`🚗 Driver: ${attrs.driver}`);
    console.log(`\n⚡ Hyperliquid-style matching: Sub-second UX!`);
    console.log(`💡 Driver and Rider can now proceed with encrypted messaging\n`);
  }

  /**
   * Handler: Request expired without match
   */
  private onRequestExpired(
    attrs: Record<string, string>,
    height: string,
    source: string
  ): void {
    console.log("\n" + "=".repeat(60));
    console.log(`⏰ REQUEST EXPIRED [${source} @ block ${height}]`);
    console.log("=".repeat(60));
    console.log(`🎫 Request ID: ${attrs.request_id}`);
    console.log(`\n💡 No drivers found within TTL, request removed from pending pool`);
  }

  /**
   * Handle WebSocket disconnect
   */
  private handleDisconnect(): void {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      console.log(
        `🔄 Reconnecting (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})...`
      );

      setTimeout(() => {
        this.connect().catch((error) => {
          console.error("Failed to reconnect:", error.message);
        });
      }, this.reconnectDelay);
    } else {
      console.error(
        `❌ Max reconnection attempts reached. Please restart the listener.`
      );
    }
  }

  /**
   * Gracefully close the connection
   */
  close(): void {
    if (this.ws) {
      console.log("\n👋 Closing event listener...");
      this.ws.close();
      this.ws = null;
    }
  }
}

// Main entry point
async function main() {
  console.log("🚀 RideHail Event Listener\n");
  console.log("Connecting to Cosmos SDK event stream...");
  console.log("This will listen for real-time matching events\n");

  const listener = new RideHailEventListener();

  try {
    await listener.connect();

    console.log("\n✅ Event listener is running!");
    console.log("📡 Listening for RideHail events:");
    console.log("   - ridehail_request_created");
    console.log("   - driver_commit_submitted");
    console.log("   - ridehail_match");
    console.log("   - ridehail_request_expired");
    console.log("\nPress Ctrl+C to stop\n");

    // Handle graceful shutdown
    process.on("SIGINT", () => {
      console.log("\n\n⏹️  Shutting down...");
      listener.close();
      process.exit(0);
    });

    process.on("SIGTERM", () => {
      listener.close();
      process.exit(0);
    });

    // Keep the process running
    await new Promise(() => {});
  } catch (error: any) {
    console.error("❌ Failed to start event listener:", error.message);
    console.error("\n💡 Make sure the node is running with:");
    console.error("   ./local_node.sh");
    process.exit(1);
  }
}

// Run the listener
main();
