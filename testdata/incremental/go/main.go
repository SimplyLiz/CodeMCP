// Package main is a test fixture for incremental indexing.
package main

import "fmt"

// Message is a greeting message.
const Message = "Hello, World!"

// User represents a user in the system.
type User struct {
	Name  string
	Email string
}

// Greet returns a greeting for the user.
func (u *User) Greet() string {
	return formatGreeting(u.Name)
}

// NewUser creates a new user with the given name and email.
func NewUser(name, email string) *User {
	return &User{
		Name:  name,
		Email: email,
	}
}

func main() {
	user := NewUser("Alice", "alice@example.com")
	greeting := user.Greet()
	fmt.Println(greeting)
	fmt.Println(Message)

	result := Add(10, 20)
	fmt.Printf("Result: %d\n", result)
}
