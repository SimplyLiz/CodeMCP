NAVIGATOR_DIR := third_party/nyx-navigator
NAVIGATOR_LIB := $(NAVIGATOR_DIR)/target/release/libnavigator.a
BIN_DIR := bin

.PHONY: build build-navigator build-fast test test-navigator lint clean check-navigator

## Build CKB with navigator integration (default)
build: build-navigator
	@mkdir -p $(BIN_DIR)
	go build -tags navigator -o $(BIN_DIR)/ckb ./cmd/ckb/...
	@echo "Built: $(BIN_DIR)/ckb (with nyx-navigator)"

## Build the nyx-navigator static library (requires Rust toolchain)
build-navigator:
	@echo "Building nyx-navigator static library..."
	@cd $(NAVIGATOR_DIR) && cargo build --release
	@echo "Localizing tree-sitter symbols (prevents link-time collisions with go-tree-sitter)..."
	@cd $(NAVIGATOR_DIR) && scripts/localize-tree-sitter-symbols.sh target/release/libnavigator.a
	@echo "Library: $(NAVIGATOR_LIB)"

## Build without navigator (no Rust toolchain required — for CI and contributors)
build-fast:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/ckb ./cmd/ckb/...
	@echo "Built: $(BIN_DIR)/ckb (without nyx-navigator)"

## Run all tests
test:
	go test ./...

## Run all tests with navigator compiled in
test-navigator: build-navigator
	go test -tags navigator ./...

## Lint
lint:
	golangci-lint run ./...

## Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## Check whether the navigator library is present
check-navigator:
	@if [ -f "$(NAVIGATOR_LIB)" ]; then \
		echo "nyx-navigator library found: $(NAVIGATOR_LIB)"; \
	else \
		echo "nyx-navigator library NOT found. Run: make build-navigator"; \
		exit 1; \
	fi
