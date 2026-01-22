/**
 * Service module - business logic with interface pattern
 */

import { Model } from "./model";
import { ValidateInput, SanitizeInput } from "../internal/util";

/**
 * Service interface defining business operations
 */
export interface Service {
  /**
   * Validates the input before processing
   * @param input The input to validate
   * @throws Error if validation fails
   */
  validate(input: string): void;

  /**
   * Processes input and returns result
   * @param input The input to process
   * @returns The processed result
   */
  process(input: string): string;
}

/**
 * Default service implementation
 */
export class DefaultService implements Service {
  private model: Model;

  constructor(model: Model) {
    this.model = model;
  }

  validate(input: string): void {
    ValidateInput(input);
  }

  process(input: string): string {
    this.validate(input);
    const clean = SanitizeInput(input);
    return this.model.transform() + ": " + clean;
  }
}

/**
 * Factory function for creating default service
 * @param model The model to use
 * @returns New DefaultService instance
 */
export function newDefaultService(model: Model): DefaultService {
  return new DefaultService(model);
}

/**
 * Caching service implementation with memoization
 */
export class CachingService implements Service {
  private inner: Service;
  private cache: Map<string, string>;

  constructor(inner: Service) {
    this.inner = inner;
    this.cache = new Map();
  }

  validate(input: string): void {
    this.inner.validate(input);
  }

  process(input: string): string {
    const cached = this.cache.get(input);
    if (cached !== undefined) {
      return cached;
    }
    const result = this.inner.process(input);
    this.cache.set(input, result);
    return result;
  }
}

/**
 * Factory function for creating caching service
 * @param inner The service to wrap
 * @returns New CachingService instance
 */
export function newCachingService(inner: Service): CachingService {
  return new CachingService(inner);
}
