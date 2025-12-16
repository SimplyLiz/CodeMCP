# Contributing to CKB

Thank you for your interest in contributing to CKB!

## Getting Started

### Prerequisites

- Go 1.21 or later
- Git
- Make (optional)

### Setup

```bash
# Clone the repository
git clone https://github.com/SimplyLiz/CodeMCP.git
cd CodeMCP

# Install dependencies
go mod download

# Build
go build -o ckb ./cmd/ckb

# Run tests
go test ./...
```

## Development Workflow

### Branch Naming

- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring

### Commit Messages

Follow conventional commits:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat` - New feature
- `fix` - Bug fix
- `docs` - Documentation
- `refactor` - Code refactoring
- `test` - Adding tests
- `chore` - Maintenance

Examples:
```
feat(api): add pagination to search endpoint
fix(cache): correct TTL calculation for negative cache
docs(readme): update installation instructions
```

### Pull Requests

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Write/update tests
5. Update documentation
6. Submit a pull request

PR checklist:
- [ ] Tests pass (`go test ./...`)
- [ ] Code builds (`go build ./...`)
- [ ] No lint errors (`go vet ./...`)
- [ ] Documentation updated
- [ ] Commit messages follow conventions

## Code Style

### Go Guidelines

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Run `go vet` before committing
- Keep functions focused and small
- Write descriptive comments for exported items

### Project Structure

```
cmd/           # CLI commands
internal/      # Internal packages (not importable)
docs/          # Documentation
```

### Error Handling

Use the internal error taxonomy:

```go
import "github.com/ckb/ckb/internal/errors"

// Return CKB errors
return errors.New(errors.SYMBOL_NOT_FOUND, "symbol not found", nil)

// Wrap errors with context
return errors.Wrap(err, errors.INTERNAL_ERROR, "failed to query backend")
```

### Logging

Use structured logging:

```go
import "github.com/ckb/ckb/internal/logging"

logger.Info("processing request",
    "symbolId", symbolId,
    "backend", backend,
)
```

## Testing

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/identity/...

# With coverage
go test -cover ./...

# Verbose
go test -v ./...
```

### Writing Tests

- Place tests in `*_test.go` files
- Use table-driven tests for multiple cases
- Test edge cases and error conditions
- Use meaningful test names

```go
func TestSymbolResolution(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid symbol",
            input:    "ckb:repo:sym:abc123",
            expected: "resolved-id",
            wantErr:  false,
        },
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Documentation

### Code Comments

- Document all exported types and functions
- Explain "why" not just "what"
- Include examples for complex APIs

```go
// ProcessSymbol processes a symbol and returns its resolved identity.
// It follows alias chains up to maxDepth levels.
//
// Example:
//
//     result, err := ProcessSymbol(ctx, "ckb:repo:sym:abc123")
//     if err != nil {
//         return err
//     }
//     fmt.Println(result.StableId)
//
func ProcessSymbol(ctx context.Context, id string) (*Symbol, error) {
    // ...
}
```

### Documentation Files

- Keep docs/ files up to date
- Use clear, concise language
- Include examples
- Update API reference for new endpoints

## Adding Features

### New CLI Command

1. Create file in `cmd/ckb/`
2. Define Cobra command
3. Add to root command
4. Update documentation

```go
// cmd/ckb/mycommand.go
var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Brief description",
    Long:  `Longer description...`,
    RunE: func(cmd *cobra.Command, args []string) error {
        // implementation
        return nil
    },
}

func init() {
    rootCmd.AddCommand(myCmd)
}
```

### New API Endpoint

1. Add handler in `internal/api/handlers.go`
2. Register route in `internal/api/routes.go`
3. Update OpenAPI spec in `internal/api/openapi.go`
4. Add tests
5. Update API documentation

### New Internal Package

1. Create directory in `internal/`
2. Add `doc.go` with package documentation
3. Implement functionality
4. Add `*_test.go` files
5. Add README.md for complex packages

## Reporting Issues

### Bug Reports

Include:
- CKB version (`ckb version`)
- Go version (`go version`)
- Operating system
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs

### Feature Requests

Include:
- Use case description
- Proposed solution
- Alternatives considered

## Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help others learn and grow

## Questions?

- Open an issue for questions
- Check existing issues first
- Tag with `question` label

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
