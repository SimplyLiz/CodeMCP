"""Utility functions for the test application."""


def greet(name: str) -> str:
    """Generate a greeting message."""
    return f"Hello, {name}!"


def calculate(a: int, b: int) -> int:
    """Perform a calculation on two numbers."""
    return a + b


def format_string(input_str: str) -> str:
    """Format a string by capitalizing the first letter."""
    if not input_str:
        return input_str
    return input_str[0].upper() + input_str[1:]


def is_valid(value: str) -> bool:
    """Validate if a string is not empty."""
    return len(value) > 0
