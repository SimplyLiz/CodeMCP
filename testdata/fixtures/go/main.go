// Package main is the entry point for the fixture application.
// This fixture tests code intelligence features across a realistic Go codebase.
package main

import (
	"fixture/pkg"
	"fixture/internal"
)

// main is the application entry point.
// Call chain: main -> RunServer -> handler.Handle -> service.Process -> internal.FormatOutput
func main() {
	server := pkg.NewServer()
	server.RunServer()
}

// Handler is a function that handles requests (disambiguation: same name as pkg.Handler type).
func Handler() string {
	return internal.FormatOutput("main handler")
}
