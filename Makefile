CARTOGRAPHER_DIR := third_party/cartographer
CARTOGRAPHER_LIB := $(CARTOGRAPHER_DIR)/target/release/libcode_cartographer.a
BIN_DIR := bin

.PHONY: build build-cartographer build-fast test test-cartographer lint clean check-cartographer

## Build CKB with cartographer integration (default)
build: build-cartographer
	@mkdir -p $(BIN_DIR)
	go build -tags cartographer -o $(BIN_DIR)/ckb ./cmd/ckb/...
	@echo "Built: $(BIN_DIR)/ckb (with cartographer)"

## Build the cartographer static library (requires Rust toolchain)
build-cartographer:
	@echo "Building cartographer static library..."
	@cd $(CARTOGRAPHER_DIR) && cargo build --release
	@echo "Localizing tree-sitter symbols (prevents link-time collisions with go-tree-sitter)..."
	@cd $(CARTOGRAPHER_DIR) && bash scripts/localize-tree-sitter-symbols.sh target/release/libcode_cartographer.a
	@echo "Library: $(CARTOGRAPHER_LIB)"

## Build without cartographer (no Rust toolchain required — for CI and contributors)
build-fast:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/ckb ./cmd/ckb/...
	@echo "Built: $(BIN_DIR)/ckb (without cartographer)"

## Run all tests
test:
	go test ./...

## Run all tests with cartographer compiled in
test-cartographer: build-cartographer
	go test -tags cartographer ./...

## Lint
lint:
	golangci-lint run ./...

## Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## Check whether the cartographer library is present
check-cartographer:
	@if [ -f "$(CARTOGRAPHER_LIB)" ]; then \
		echo "cartographer library found: $(CARTOGRAPHER_LIB)"; \
	else \
		echo "cartographer library NOT found. Run: make build-cartographer"; \
		exit 1; \
	fi
