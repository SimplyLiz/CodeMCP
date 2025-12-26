import { greet, calculate, formatString } from './utils';

/**
 * Main entry point for the test application.
 */
function main(): void {
  const greeting = greet('World');
  console.log(greeting);

  const result = calculate(10, 5);
  console.log(`Result: ${result}`);
}

/**
 * Processes user input and returns a formatted string.
 */
export function processInput(input: string): string {
  return formatString(input.trim());
}

main();
