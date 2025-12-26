/**
 * Handler module - request handling
 */

import { Service } from "./service";
import { FormatOutput } from "../internal/util";

/**
 * Handler class for processing requests
 */
export class Handler {
  private service: Service;

  constructor(service: Service) {
    this.service = service;
  }

  /**
   * Handles a single request
   * @param input The input to process
   * @returns The formatted output
   */
  handle(input: string): string {
    const result = this.service.process(input);
    return FormatOutput(result);
  }

  /**
   * Handles a batch of requests
   * @param inputs The inputs to process
   * @returns Array of results
   */
  handleBatch(inputs: string[]): string[] {
    return inputs.map((input) => this.handle(input));
  }
}

/**
 * Factory function for creating handlers
 * @param service The service to use
 * @returns New Handler instance
 */
export function newHandler(service: Service): Handler {
  return new Handler(service);
}

/**
 * Standalone handler function (for disambiguation testing)
 * Demonstrates handler() function vs Handler class
 * @param input The input to process
 * @returns The processed string
 */
export function handler(input: string): string {
  return `handled: ${input}`;
}
