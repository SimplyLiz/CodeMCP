/**
 * Generates a greeting message.
 */
export function greet(name: string): string {
  return `Hello, ${name}!`;
}

/**
 * Performs a calculation on two numbers.
 */
export function calculate(a: number, b: number): number {
  return a + b;
}

/**
 * Formats a string by capitalizing the first letter.
 */
export function formatString(input: string): string {
  if (input.length === 0) return input;
  return input.charAt(0).toUpperCase() + input.slice(1);
}

/**
 * Validates if a string is not empty.
 */
export function isValid(value: string): boolean {
  return value.length > 0;
}
