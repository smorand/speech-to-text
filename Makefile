.PHONY: build install uninstall clean clean-all rebuild test fmt vet

# Binary name
BINARY_NAME=speech-to-text
CMD_DIR=./cmd/speech-to-text
BUILD_DIR=bin

# Source files
SOURCES=$(shell find cmd internal -name '*.go' 2>/dev/null)

# Build target
build: $(BUILD_DIR)/$(BINARY_NAME)

rebuild: clean build

$(BUILD_DIR)/$(BINARY_NAME): $(SOURCES) go.sum
	@mkdir -p $(BUILD_DIR)
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete! Binary: $(BUILD_DIR)/$(BINARY_NAME)"

# Generate go.sum
go.sum: go.mod
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies downloaded"


# Install binary
install: build
ifndef TARGET
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "Installation complete!"
else
	@echo "Installing $(BINARY_NAME) to $(TARGET)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(TARGET)/ 2>/dev/null || sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(TARGET)/
	@echo "Installation complete!"
endif

# Uninstall binary
uninstall:
	@echo "Looking for $(BINARY_NAME) in system..."
	@BINARY_PATH=$$(which $(BINARY_NAME) 2>/dev/null); \
	if [ -z "$$BINARY_PATH" ]; then \
		echo "$(BINARY_NAME) not found in PATH"; \
		exit 0; \
	fi; \
	if [ -f "$$BINARY_PATH" ]; then \
		if [ "$$(basename $$(dirname $$BINARY_PATH))" = "bin" ]; then \
			echo "Found $(BINARY_NAME) at $$BINARY_PATH"; \
			echo "Removing..."; \
			sudo rm -f "$$BINARY_PATH"; \
			echo "Uninstallation complete!"; \
		else \
			echo "$(BINARY_NAME) found at $$BINARY_PATH but not in a standard bin directory"; \
			echo "Please remove it manually if needed"; \
		fi; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete!"

clean-all: clean
	@echo "Cleaning dependencies cache..."
	@go clean -modcache
	@echo "Clean complete!"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format complete!"

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete!"

# Run the application
run:
	@go run $(CMD_DIR) $(ARGS)

# Build for multiple platforms
build-all:
	@mkdir -p $(BUILD_DIR)
	@echo "Building for all platforms..."
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	@GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	@GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@echo "All builds complete in $(BUILD_DIR)/"

# Run all checks (fmt, vet, test)
check: fmt vet test
	@echo "All checks passed!"

# Help
help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  rebuild    - Clean and rebuild from scratch"
	@echo "  install    - Build and install to /usr/local/bin (or TARGET env variable)"
	@echo "  uninstall  - Remove installed binary"
	@echo "  clean      - Remove build artifacts"
	@echo "  clean-all  - Remove build artifacts and clean module cache"
	@echo "  test       - Run tests"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  run        - Run the application (use ARGS to pass arguments)"
	@echo "  build-all  - Build for all platforms (linux, darwin, windows)"
	@echo "  check      - Run fmt, vet, and test"
	@echo "  help       - Show this help message"
