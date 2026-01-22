import 'utils.dart';

/// Main entry point for the test application.
void main() {
  final greeting = greet('World');
  print(greeting);

  final result = calculate(10, 5);
  print('Result: $result');
}

/// Processes user input and returns a formatted string.
String processInput(String input) {
  return formatString(input.trim());
}
