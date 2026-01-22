/**
 * Server module - HTTP server wrapper
 */

import { Handler } from "./handler";

/**
 * Server wraps a handler and provides server-like interface
 */
export class Server {
  private handler: Handler;

  constructor(handler: Handler) {
    this.handler = handler;
  }

  /**
   * Gets the underlying handler
   * @returns The handler instance
   */
  getHandler(): Handler {
    return this.handler;
  }

  /**
   * Runs the server (simulated)
   * In a real implementation, this would start an HTTP server
   */
  runServer(): void {
    console.log("Server running...");
    // Simulate server loop
    const input = "test";
    const result = this.handler.handle(input);
    console.log("Processed:", result);
  }
}

/**
 * Factory function for creating servers
 * @param handler The handler to use
 * @returns New Server instance
 */
export function newServer(handler: Handler): Server {
  return new Server(handler);
}
