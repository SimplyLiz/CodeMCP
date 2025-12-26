/**
 * Main entry point
 * Demonstrates full application wiring
 */

import { newModel } from "./pkg/model";
import { newDefaultService, newCachingService } from "./pkg/service";
import { newHandler, handler } from "./pkg/handler";
import { newServer } from "./pkg/server";
import { FormatOutput } from "./internal/util";

/**
 * Main handler function - application entry point
 * @param input The input to process
 * @returns The processed result
 */
export function main(input: string): string {
  // Create model with config
  const model = newModel("fixture");
  model.setConfig({
    prefix: "[",
    uppercase: true,
  });

  // Build service chain with caching
  const baseService = newDefaultService(model);
  const cachingService = newCachingService(baseService);

  // Create handler
  const h = newHandler(cachingService);

  // Process input
  const result = h.handle(input);

  // Use standalone handler function (disambiguation test)
  const standaloneResult = handler(input);

  // Format and combine
  return FormatOutput(`${result} | ${standaloneResult}`, "main");
}

/**
 * Alternative entry point using server
 */
export function runServer(): void {
  const model = newModel("server");
  const service = newDefaultService(model);
  const h = newHandler(service);
  const server = newServer(h);
  server.runServer();
}

// Execute if run directly
if (require.main === module) {
  console.log(main("hello world"));
}
