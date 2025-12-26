/**
 * Model module - data structures
 */

/**
 * Configuration for model transformation
 */
export interface Config {
  /** Prefix to add to output */
  prefix: string;
  /** Whether to convert to uppercase */
  uppercase: boolean;
}

/**
 * Default configuration
 * @returns Default Config with empty prefix and lowercase
 */
export function defaultConfig(): Config {
  return {
    prefix: "",
    uppercase: false,
  };
}

/**
 * Core data model
 */
export class Model {
  /** Model name */
  public name: string;
  /** Model configuration */
  public config: Config;

  constructor(name: string) {
    this.name = name;
    this.config = defaultConfig();
  }

  /**
   * Creates a copy of the model
   * @returns New Model with same values
   */
  clone(): Model {
    const m = new Model(this.name);
    m.config = { ...this.config };
    return m;
  }

  /**
   * Sets the model configuration
   * @param cfg The new configuration
   */
  setConfig(cfg: Config): void {
    this.config = cfg;
  }

  /**
   * Transforms the model name based on config
   * @returns The transformed name
   */
  transform(): string {
    let result = this.name;
    if (this.config.uppercase) {
      result = result.toUpperCase();
    }
    if (this.config.prefix) {
      result = `${this.config.prefix}${result}`;
    }
    return result;
  }
}

/**
 * Factory function for creating models
 * @param name The model name
 * @returns New Model instance
 */
export function newModel(name: string): Model {
  return new Model(name);
}
