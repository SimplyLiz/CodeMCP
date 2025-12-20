//go:build cgo

package symbols

import (
	"context"
	"testing"

	"ckb/internal/complexity"
)

func TestExtractSource_Go(t *testing.T) {
	source := []byte(`package main

type Handler struct {
	db *Database
}

func NewHandler(db *Database) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Get(id string) (*Item, error) {
	return h.db.Find(id)
}

func helper() {
	// private helper
}
`)

	e := NewExtractor()
	if e == nil {
		t.Skip("tree-sitter not available")
	}

	symbols, err := e.ExtractSource(context.Background(), "test.go", source, complexity.LangGo)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find: Handler (type), NewHandler (function), Get (method), helper (function)
	if len(symbols) < 4 {
		t.Errorf("expected at least 4 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  %s: %s (%s)", s.Kind, s.Name, s.Container)
		}
	}

	// Verify we found the type
	found := false
	for _, s := range symbols {
		if s.Name == "Handler" && s.Kind == "type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find Handler type")
	}

	// Verify we found NewHandler function
	found = false
	for _, s := range symbols {
		if s.Name == "NewHandler" && s.Kind == "function" {
			found = true
			if s.Confidence != 0.7 {
				t.Errorf("expected confidence 0.7, got %v", s.Confidence)
			}
			if s.Source != "treesitter" {
				t.Errorf("expected source 'treesitter', got %s", s.Source)
			}
			break
		}
	}
	if !found {
		t.Error("did not find NewHandler function")
	}

	// Verify we found Get method
	found = false
	for _, s := range symbols {
		if s.Name == "Get" && s.Kind == "method" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find Get method")
	}
}

func TestExtractSource_TypeScript(t *testing.T) {
	source := []byte(`
interface UserService {
	getUser(id: string): Promise<User>;
}

class UserServiceImpl implements UserService {
	constructor(private db: Database) {}

	async getUser(id: string): Promise<User> {
		return this.db.find(id);
	}

	private validate(user: User): boolean {
		return user.id !== "";
	}
}

function createService(db: Database): UserService {
	return new UserServiceImpl(db);
}

const helper = (x: number) => x * 2;
`)

	e := NewExtractor()
	if e == nil {
		t.Skip("tree-sitter not available")
	}

	symbols, err := e.ExtractSource(context.Background(), "test.ts", source, complexity.LangTypeScript)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find interface, class, methods, function, arrow function
	if len(symbols) < 5 {
		t.Errorf("expected at least 5 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  %s: %s (%s)", s.Kind, s.Name, s.Container)
		}
	}

	// Check for interface
	found := false
	for _, s := range symbols {
		if s.Name == "UserService" && s.Kind == "interface" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find UserService interface")
	}

	// Check for class
	found = false
	for _, s := range symbols {
		if s.Name == "UserServiceImpl" && s.Kind == "class" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find UserServiceImpl class")
	}

	// Check for function
	found = false
	for _, s := range symbols {
		if s.Name == "createService" && s.Kind == "function" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find createService function")
	}
}

func TestExtractSource_Python(t *testing.T) {
	source := []byte(`
class UserRepository:
    def __init__(self, db):
        self.db = db

    def get_user(self, user_id: str) -> User:
        return self.db.find(user_id)

    def _validate(self, user):
        return user.id is not None

def create_repository(db) -> UserRepository:
    return UserRepository(db)
`)

	e := NewExtractor()
	if e == nil {
		t.Skip("tree-sitter not available")
	}

	symbols, err := e.ExtractSource(context.Background(), "test.py", source, complexity.LangPython)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find class, methods, function
	if len(symbols) < 4 {
		t.Errorf("expected at least 4 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  %s: %s (%s)", s.Kind, s.Name, s.Container)
		}
	}

	// Check for class
	found := false
	for _, s := range symbols {
		if s.Name == "UserRepository" && s.Kind == "class" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find UserRepository class")
	}

	// Check for function
	found = false
	for _, s := range symbols {
		if s.Name == "create_repository" && s.Kind == "function" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find create_repository function")
	}
}

func TestExtractSource_Rust(t *testing.T) {
	source := []byte(`
struct Handler {
    db: Database,
}

impl Handler {
    fn new(db: Database) -> Self {
        Handler { db }
    }

    fn get(&self, id: &str) -> Option<Item> {
        self.db.find(id)
    }
}

trait Service {
    fn process(&self, data: &[u8]) -> Result<(), Error>;
}

fn helper() -> bool {
    true
}
`)

	e := NewExtractor()
	if e == nil {
		t.Skip("tree-sitter not available")
	}

	symbols, err := e.ExtractSource(context.Background(), "test.rs", source, complexity.LangRust)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find struct, impl methods, trait, function
	if len(symbols) < 4 {
		t.Errorf("expected at least 4 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  %s: %s (%s)", s.Kind, s.Name, s.Container)
		}
	}

	// Check for struct
	found := false
	for _, s := range symbols {
		if s.Name == "Handler" && s.Kind == "type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find Handler struct")
	}

	// Check for trait
	found = false
	for _, s := range symbols {
		if s.Name == "Service" && s.Kind == "interface" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find Service trait")
	}

	// Check for helper function
	found = false
	for _, s := range symbols {
		if s.Name == "helper" && s.Kind == "function" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find helper function")
	}
}

func TestExtractSource_Java(t *testing.T) {
	source := []byte(`
package com.example;

interface UserService {
    User getUser(String id);
}

class UserServiceImpl implements UserService {
    private Database db;

    public UserServiceImpl(Database db) {
        this.db = db;
    }

    @Override
    public User getUser(String id) {
        return db.find(id);
    }

    private boolean validate(User user) {
        return user.getId() != null;
    }
}
`)

	e := NewExtractor()
	if e == nil {
		t.Skip("tree-sitter not available")
	}

	symbols, err := e.ExtractSource(context.Background(), "test.java", source, complexity.LangJava)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	// Should find interface, class, methods
	if len(symbols) < 4 {
		t.Errorf("expected at least 4 symbols, got %d", len(symbols))
		for _, s := range symbols {
			t.Logf("  %s: %s (%s)", s.Kind, s.Name, s.Container)
		}
	}

	// Check for interface
	found := false
	for _, s := range symbols {
		if s.Name == "UserService" && s.Kind == "interface" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find UserService interface")
	}

	// Check for class
	found = false
	for _, s := range symbols {
		if s.Name == "UserServiceImpl" && s.Kind == "class" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find UserServiceImpl class")
	}
}

func TestSymbolMetadata(t *testing.T) {
	source := []byte(`package main

func ProcessData(input []byte) ([]byte, error) {
	// line 3-5
	return nil, nil
}
`)

	e := NewExtractor()
	if e == nil {
		t.Skip("tree-sitter not available")
	}

	symbols, err := e.ExtractSource(context.Background(), "test.go", source, complexity.LangGo)
	if err != nil {
		t.Fatalf("ExtractSource failed: %v", err)
	}

	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}

	s := symbols[0]
	if s.Name != "ProcessData" {
		t.Errorf("expected name ProcessData, got %s", s.Name)
	}
	if s.Kind != "function" {
		t.Errorf("expected kind function, got %s", s.Kind)
	}
	if s.Path != "test.go" {
		t.Errorf("expected path test.go, got %s", s.Path)
	}
	if s.Line != 3 {
		t.Errorf("expected line 3, got %d", s.Line)
	}
	if s.Source != "treesitter" {
		t.Errorf("expected source treesitter, got %s", s.Source)
	}
	if s.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %v", s.Confidence)
	}
	if s.Signature == "" {
		t.Error("expected non-empty signature")
	}
}

func TestIsAvailable(t *testing.T) {
	// This test runs in CGO mode, so should be true
	if !IsAvailable() {
		t.Error("expected IsAvailable() to be true with CGO")
	}
}
