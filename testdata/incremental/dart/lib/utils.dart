/// Generates a greeting message.
String greet(String name) {
  return 'Hello, $name!';
}

/// Performs a calculation on two numbers.
int calculate(int a, int b) {
  return a + b;
}

/// Formats a string by capitalizing the first letter.
String formatString(String input) {
  if (input.isEmpty) return input;
  return input[0].toUpperCase() + input.substring(1);
}

/// Validates if a string is not empty.
bool isValid(String value) {
  return value.isNotEmpty;
}
