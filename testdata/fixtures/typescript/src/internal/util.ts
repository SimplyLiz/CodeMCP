/**
 * Internal utilities module
 */

/** Error for empty input validation */
export class EmptyInputError extends Error {
  constructor() {
    super("input cannot be empty");
    this.name = "EmptyInputError";
  }
}

/**
 * Validates that input is not empty
 * @param input The string to validate
 * @throws EmptyInputError if input is empty
 */
export function ValidateInput(input: string): void {
  if (input.trim() === "") {
    throw new EmptyInputError();
  }
}

/**
 * Sanitizes input by trimming whitespace
 * @param input The string to sanitize
 * @returns The sanitized string
 */
export function SanitizeInput(input: string): string {
  return input.trim();
}

/**
 * Formats output with optional prefix
 * @param value The value to format
 * @param prefix Optional prefix to prepend
 * @returns The formatted string
 */
export function FormatOutput(value: string, prefix?: string): string {
  if (prefix) {
    return `${prefix}: ${value}`;
  }
  return value;
}
