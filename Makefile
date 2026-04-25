CARTOGRAPHER_DIR := third_party/cartographer/mapper-core/cartographer
CARTOGRAPHER_LIB := $(CARTOGRAPHER_DIR)/target/release/libcartographer.a
BIN_DIR := bin

.PHONY: build build-cartographer build-fast test test-cartographer lint clean

## Build CKB with Cartographer integration (default)
build: build-cartographer
	@mkdir -p $(BIN_DIR)
	go build -tags cartographer -o $(BIN_DIR)/ckb ./cmd/ckb/...
	@echo "Built: $(BIN_DIR)/ckb (with Cartographer)"

## Build the Cartographer static library (requires Rust toolchain)
build-cartographer:
	@echo "Building Cartographer static library..."
	@cd $(CARTOGRAPHER_DIR) && cargo build --release
	@echo "Localizing tree-sitter symbols (prevents link-time collisions with go-tree-sitter)..."
	@cd $(CARTOGRAPHER_DIR) && scripts/localize-tree-sitter-symbols.sh target/release/libcartographer.a
	@echo "Library: $(CARTOGRAPHER_LIB)"

## Build without Cartographer (no Rust toolchain required — for CI and contributors)
build-fast:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/ckb ./cmd/ckb/...
	@echo "Built: $(BIN_DIR)/ckb (without Cartographer)"

## Run all tests
test:
	go test ./...

## Run all tests with Cartographer compiled in
test-cartographer: build-cartographer
	go test -tags cartographer ./...

## Lint
lint:
	golangci-lint run ./...

## Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## Check whether the Cartographer library is present
check-cartographer:
	@if [ -f "$(CARTOGRAPHER_LIB)" ]; then \
		echo "Cartographer library found: $(CARTOGRAPHER_LIB)"; \
	else \
		echo "Cartographer library NOT found. Run: make build-cartographer"; \
		exit 1; \
	fi
