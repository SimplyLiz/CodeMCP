package symbols

import (
	"context"
	"testing"

	"ckb/internal/complexity"
	"ckb/internal/logging"
)

func testLogger() *logging.Logger {
	return logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})
}

func TestExtractGoFunctions(t *testing.T) {
	source := []byte(`package main

func Hello() string {
	return "hello"
}

func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request) {
	// handle request
}

type Server struct {
	port int
}
`)

	logger := testLogger()
	extractor := NewExtractor(logger)

	symbols, err := extractor.ExtractSource(context.Background(), "test.go", source, complexity.LangGo)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find: Hello (function), HandleRequest (method), Server (type)
	if len(symbols) < 3 {
		t.Errorf("Expected at least 3 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  - %s (%s)", s.Name, s.Kind)
		}
	}

	// Check Hello function
	var hello *Symbol
	for i := range symbols {
		if symbols[i].Name == "Hello" {
			hello = &symbols[i]
			break
		}
	}
	if hello == nil {
		t.Error("Expected to find Hello function")
	} else {
		if hello.Kind != KindFunction {
			t.Errorf("Expected Hello to be function, got %s", hello.Kind)
		}
	}

	// Check HandleRequest method
	var handleReq *Symbol
	for i := range symbols {
		if symbols[i].Name == "HandleRequest" {
			handleReq = &symbols[i]
			break
		}
	}
	if handleReq == nil {
		t.Error("Expected to find HandleRequest method")
	} else {
		if handleReq.Kind != KindMethod {
			t.Errorf("Expected HandleRequest to be method, got %s", handleReq.Kind)
		}
		if handleReq.ContainerName != "Server" {
			t.Errorf("Expected HandleRequest container to be Server, got %s", handleReq.ContainerName)
		}
	}

	// Check Server type
	var server *Symbol
	for i := range symbols {
		if symbols[i].Name == "Server" {
			server = &symbols[i]
			break
		}
	}
	if server == nil {
		t.Error("Expected to find Server type")
	} else {
		if server.Kind != KindType {
			t.Errorf("Expected Server to be type, got %s", server.Kind)
		}
	}
}

func TestExtractTypeScriptSymbols(t *testing.T) {
	source := []byte(`
class UserService {
	private users: Map<string, User>;

	constructor() {
		this.users = new Map();
	}

	getUser(id: string): User | undefined {
		return this.users.get(id);
	}
}

interface User {
	id: string;
	name: string;
}

function createUser(name: string): User {
	return { id: crypto.randomUUID(), name };
}
`)

	logger := testLogger()
	extractor := NewExtractor(logger)

	symbols, err := extractor.ExtractSource(context.Background(), "test.ts", source, complexity.LangTypeScript)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find: UserService (class), getUser (method), User (interface), createUser (function)
	if len(symbols) < 4 {
		t.Errorf("Expected at least 4 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  - %s (%s)", s.Name, s.Kind)
		}
	}

	// Check class
	var userService *Symbol
	for i := range symbols {
		if symbols[i].Name == "UserService" {
			userService = &symbols[i]
			break
		}
	}
	if userService == nil {
		t.Error("Expected to find UserService class")
	} else if userService.Kind != KindClass {
		t.Errorf("Expected UserService to be class, got %s", userService.Kind)
	}

	// Check interface
	var user *Symbol
	for i := range symbols {
		if symbols[i].Name == "User" {
			user = &symbols[i]
			break
		}
	}
	if user == nil {
		t.Error("Expected to find User interface")
	} else if user.Kind != KindInterface {
		t.Errorf("Expected User to be interface, got %s", user.Kind)
	}
}

func TestExtractPythonSymbols(t *testing.T) {
	source := []byte(`
class Calculator:
    def __init__(self):
        self.result = 0

    def add(self, x, y):
        return x + y

def main():
    calc = Calculator()
    print(calc.add(1, 2))
`)

	logger := testLogger()
	extractor := NewExtractor(logger)

	symbols, err := extractor.ExtractSource(context.Background(), "test.py", source, complexity.LangPython)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find: Calculator (class), __init__ (method), add (method), main (function)
	if len(symbols) < 4 {
		t.Errorf("Expected at least 4 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  - %s (%s)", s.Name, s.Kind)
		}
	}

	// Check main function (should be regular function, not method)
	var main *Symbol
	for i := range symbols {
		if symbols[i].Name == "main" {
			main = &symbols[i]
			break
		}
	}
	if main == nil {
		t.Error("Expected to find main function")
	} else if main.Kind != KindFunction {
		t.Errorf("Expected main to be function, got %s", main.Kind)
	}

	// Check add method
	var add *Symbol
	for i := range symbols {
		if symbols[i].Name == "add" {
			add = &symbols[i]
			break
		}
	}
	if add == nil {
		t.Error("Expected to find add method")
	} else {
		if add.Kind != KindMethod {
			t.Errorf("Expected add to be method, got %s", add.Kind)
		}
		if add.ContainerName != "Calculator" {
			t.Errorf("Expected add container to be Calculator, got %s", add.ContainerName)
		}
	}
}

func TestSearch(t *testing.T) {
	logger := testLogger()
	extractor := NewExtractor(logger)

	// Search in the current codebase (internal/symbols)
	symbols, err := extractor.Search(context.Background(), ".", "Extract", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should find some functions containing "Extract"
	if len(symbols) == 0 {
		t.Log("No symbols found matching 'Extract' - this may be expected in test environment")
	}

	for _, s := range symbols {
		t.Logf("Found: %s (%s) at %s:%d", s.Name, s.Kind, s.Path, s.StartLine)
	}
}
