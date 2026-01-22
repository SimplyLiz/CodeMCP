"""Main module for the test application."""

from .utils import greet, calculate, format_string


def main() -> None:
    """Main entry point for the test application."""
    greeting = greet("World")
    print(greeting)

    result = calculate(10, 5)
    print(f"Result: {result}")


def process_input(input_str: str) -> str:
    """Process user input and return a formatted string."""
    return format_string(input_str.strip())


if __name__ == "__main__":
    main()
