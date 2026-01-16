.PHONY: build build-all install uninstall clean clean-all rebuild test fmt vet check help docker-auth docker-build plan deploy undeploy init-plan init-deploy init-destroy terraform-help check-init update-backend

# Binary name derived from current directory
BINARY_NAME=$(shell basename $$(pwd))

# Detect current platform
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)
CURRENT_PLATFORM=$(GOOS)-$(GOARCH)

# Auto-detect project structure
HAS_SRC_DIR=$(shell [ -d src ] && echo "yes" || echo "no")
HAS_CMD_DIR=$(shell [ -d cmd ] && echo "yes" || echo "no")

# Set directories based on project structure
ifeq ($(HAS_SRC_DIR),yes)
	SRC_DIR=src
	CMD_PATH=$(SRC_DIR)
	BUILD_DIR=bin
	GO_MOD_PATH=$(SRC_DIR)/go.mod
	GO_SUM_PATH=$(SRC_DIR)/go.sum
else ifeq ($(HAS_CMD_DIR),yes)
	SRC_DIR=.
	CMD_PATH=./cmd/$(BINARY_NAME)
	BUILD_DIR=bin
	GO_MOD_PATH=go.mod
	GO_SUM_PATH=go.sum
else
	SRC_DIR=.
	CMD_PATH=.
	BUILD_DIR=bin
	GO_MOD_PATH=go.mod
	GO_SUM_PATH=go.sum
endif

# Platform-specific binary names
BINARY_LINUX=$(BUILD_DIR)/$(BINARY_NAME)-linux-amd64
BINARY_DARWIN_INTEL=$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64
BINARY_DARWIN_ARM=$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64
CURRENT_BINARY=$(BUILD_DIR)/$(BINARY_NAME)-$(CURRENT_PLATFORM)
LAUNCHER_SCRIPT=$(BUILD_DIR)/$(BINARY_NAME).sh

# Build for current platform only
build: $(CURRENT_BINARY)

# Build for all platforms and create launcher script
build-all: $(BINARY_LINUX) $(BINARY_DARWIN_INTEL) $(BINARY_DARWIN_ARM) $(LAUNCHER_SCRIPT)

rebuild: clean-all build

# Build targets for each platform
$(BINARY_LINUX): $(GO_SUM_PATH)
	@echo "Building $(BINARY_NAME) for Linux AMD64..."
	@mkdir -p $(BUILD_DIR)
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && GOOS=linux GOARCH=amd64 go build -o ../$(BINARY_LINUX) .
else
	@GOOS=linux GOARCH=amd64 go build -o $(BINARY_LINUX) $(CMD_PATH)
endif
	@echo "✓ Built: $(BINARY_LINUX)"

$(BINARY_DARWIN_INTEL): $(GO_SUM_PATH)
	@echo "Building $(BINARY_NAME) for macOS Intel (AMD64)..."
	@mkdir -p $(BUILD_DIR)
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && GOOS=darwin GOARCH=amd64 go build -o ../$(BINARY_DARWIN_INTEL) .
else
	@GOOS=darwin GOARCH=amd64 go build -o $(BINARY_DARWIN_INTEL) $(CMD_PATH)
endif
	@echo "✓ Built: $(BINARY_DARWIN_INTEL)"

$(BINARY_DARWIN_ARM): $(GO_SUM_PATH)
	@echo "Building $(BINARY_NAME) for macOS Apple Silicon (ARM64)..."
	@mkdir -p $(BUILD_DIR)
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && GOOS=darwin GOARCH=arm64 go build -o ../$(BINARY_DARWIN_ARM) .
else
	@GOOS=darwin GOARCH=arm64 go build -o $(BINARY_DARWIN_ARM) $(CMD_PATH)
endif
	@echo "✓ Built: $(BINARY_DARWIN_ARM)"

# Create launcher script
$(LAUNCHER_SCRIPT): $(BINARY_LINUX) $(BINARY_DARWIN_INTEL) $(BINARY_DARWIN_ARM)
	@echo "Creating launcher script..."
	@mkdir -p $(BUILD_DIR)
	@echo '#!/bin/bash' > $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Auto-generated launcher script for $(BINARY_NAME)' >> $(LAUNCHER_SCRIPT)
	@echo '# Detects platform and executes the correct binary' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Get the directory where this script is located' >> $(LAUNCHER_SCRIPT)
	@echo 'SCRIPT_DIR="$$(cd "$$(dirname "$${BASH_SOURCE[0]}")" && pwd)"' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Detect OS' >> $(LAUNCHER_SCRIPT)
	@echo 'OS=$$(uname -s | tr "[:upper:]" "[:lower:]")' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Detect architecture' >> $(LAUNCHER_SCRIPT)
	@echo 'ARCH=$$(uname -m)' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Map architecture names to Go convention' >> $(LAUNCHER_SCRIPT)
	@echo 'case "$$ARCH" in' >> $(LAUNCHER_SCRIPT)
	@echo '    x86_64)' >> $(LAUNCHER_SCRIPT)
	@echo '        ARCH="amd64"' >> $(LAUNCHER_SCRIPT)
	@echo '        ;;' >> $(LAUNCHER_SCRIPT)
	@echo '    aarch64)' >> $(LAUNCHER_SCRIPT)
	@echo '        ARCH="arm64"' >> $(LAUNCHER_SCRIPT)
	@echo '        ;;' >> $(LAUNCHER_SCRIPT)
	@echo '    arm64)' >> $(LAUNCHER_SCRIPT)
	@echo '        ARCH="arm64"' >> $(LAUNCHER_SCRIPT)
	@echo '        ;;' >> $(LAUNCHER_SCRIPT)
	@echo '    *)' >> $(LAUNCHER_SCRIPT)
	@echo '        echo "Unsupported architecture: $$ARCH" >&2' >> $(LAUNCHER_SCRIPT)
	@echo '        exit 1' >> $(LAUNCHER_SCRIPT)
	@echo '        ;;' >> $(LAUNCHER_SCRIPT)
	@echo 'esac' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Construct binary name' >> $(LAUNCHER_SCRIPT)
	@echo 'BINARY="$$SCRIPT_DIR/$(BINARY_NAME)-$$OS-$$ARCH"' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Check if binary exists' >> $(LAUNCHER_SCRIPT)
	@echo 'if [ ! -f "$$BINARY" ]; then' >> $(LAUNCHER_SCRIPT)
	@echo '    echo "Error: Binary not found for platform $$OS-$$ARCH" >&2' >> $(LAUNCHER_SCRIPT)
	@echo '    echo "Expected: $$BINARY" >&2' >> $(LAUNCHER_SCRIPT)
	@echo '    echo "" >&2' >> $(LAUNCHER_SCRIPT)
	@echo '    echo "Available binaries:" >&2' >> $(LAUNCHER_SCRIPT)
	@echo '    ls -1 "$$SCRIPT_DIR"/$(BINARY_NAME)-* 2>/dev/null | sed "s|^|  |" >&2' >> $(LAUNCHER_SCRIPT)
	@echo '    exit 1' >> $(LAUNCHER_SCRIPT)
	@echo 'fi' >> $(LAUNCHER_SCRIPT)
	@echo '' >> $(LAUNCHER_SCRIPT)
	@echo '# Execute the binary with all arguments' >> $(LAUNCHER_SCRIPT)
	@echo 'exec "$$BINARY" "$$@"' >> $(LAUNCHER_SCRIPT)
	@chmod +x $(LAUNCHER_SCRIPT)
	@echo "✓ Created launcher script: $(LAUNCHER_SCRIPT)"

# Generate go.sum
$(GO_SUM_PATH): $(GO_MOD_PATH)
	@echo "Downloading dependencies..."
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && go mod download
	@cd $(SRC_DIR) && go mod tidy
	@touch $(GO_SUM_PATH)
else
	@go mod download
	@go mod tidy
	@touch $(GO_SUM_PATH)
endif
	@echo "Dependencies downloaded"

# Generate go.mod (only if it doesn't exist)
$(GO_MOD_PATH):
	@echo "Initializing Go module..."
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && go mod init $(BINARY_NAME)
else
	@go mod init $(BINARY_NAME)
endif

# Install binary (installs the current platform binary)
install: build
	@if [ ! -f "$(CURRENT_BINARY)" ]; then \
		echo "Error: Binary for current platform ($(CURRENT_PLATFORM)) not found"; \
		echo "Run 'make build' or 'make build-all' first"; \
		exit 1; \
	fi
ifndef TARGET
	@echo "Installing $(BINARY_NAME) ($(CURRENT_PLATFORM)) to /usr/local/bin..."
	@sudo cp $(CURRENT_BINARY) /usr/local/bin/$(BINARY_NAME)
	@echo "Installation complete!"
else
	@echo "Installing $(BINARY_NAME) ($(CURRENT_PLATFORM)) to $(TARGET)..."
	@cp $(CURRENT_BINARY) $(TARGET)/$(BINARY_NAME) 2>/dev/null || sudo cp $(CURRENT_BINARY) $(TARGET)/$(BINARY_NAME)
	@echo "Installation complete!"
endif

# Install launcher script (for multi-platform distribution)
install-launcher: build-all
ifndef TARGET
	@echo "Installing launcher script to /usr/local/bin/$(BINARY_NAME)..."
	@sudo cp $(LAUNCHER_SCRIPT) /usr/local/bin/$(BINARY_NAME)
	@echo "Installing platform binaries to /usr/local/lib/$(BINARY_NAME)/..."
	@sudo mkdir -p /usr/local/lib/$(BINARY_NAME)
	@sudo cp $(BINARY_LINUX) /usr/local/lib/$(BINARY_NAME)/
	@sudo cp $(BINARY_DARWIN_INTEL) /usr/local/lib/$(BINARY_NAME)/
	@sudo cp $(BINARY_DARWIN_ARM) /usr/local/lib/$(BINARY_NAME)/
	@echo "Installation complete!"
else
	@echo "Installing launcher script to $(TARGET)/$(BINARY_NAME)..."
	@cp $(LAUNCHER_SCRIPT) $(TARGET)/$(BINARY_NAME) 2>/dev/null || sudo cp $(LAUNCHER_SCRIPT) $(TARGET)/$(BINARY_NAME)
	@echo "Note: Platform binaries remain in $(BUILD_DIR)/"
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
			if [ -d "/usr/local/lib/$(BINARY_NAME)" ]; then \
				echo "Removing platform binaries..."; \
				sudo rm -rf "/usr/local/lib/$(BINARY_NAME)"; \
			fi; \
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

# Clean all (including go.mod and go.sum)
clean-all: clean
	@echo "Cleaning go.mod & go.sum..."
	@rm -f $(GO_MOD_PATH) $(GO_SUM_PATH)
	@echo "Clean complete!"

# Run tests
test:
	@echo "Running tests..."
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && go test -v ./...
else
	@go test -v ./...
endif

# Format code
fmt:
	@echo "Formatting code..."
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && go fmt ./...
else
	@go fmt ./...
endif
	@echo "Format complete!"

# Run go vet
vet:
	@echo "Running go vet..."
ifeq ($(HAS_SRC_DIR),yes)
	@cd $(SRC_DIR) && go vet ./...
else
	@go vet ./...
endif
	@echo "Vet complete!"

# Run all checks (fmt, vet, test)
check: fmt vet test
	@echo "All checks passed!"

# Show current platform info
info:
	@echo "Current platform: $(CURRENT_PLATFORM)"
	@echo "Binary name: $(BINARY_NAME)"
	@echo "Build directory: $(BUILD_DIR)"
	@echo "Current binary: $(CURRENT_BINARY)"

# Help
help:
	@echo "Available targets:"
	@echo "  build           - Build the binary for current platform ($(CURRENT_PLATFORM))"
	@echo "  build-all       - Build for all platforms and create launcher script"
	@echo "  rebuild         - Clean all and rebuild from scratch"
	@echo "  install         - Install current platform binary to /usr/local/bin (or TARGET)"
	@echo "  install-launcher - Install launcher script with all platform binaries"
	@echo "  uninstall       - Remove installed binary"
	@echo "  clean           - Remove build artifacts"
	@echo "  clean-all       - Remove build artifacts, go.mod, and go.sum"
	@echo "  test            - Run tests"
	@echo "  fmt             - Format code"
	@echo "  vet             - Run go vet"
	@echo "  check           - Run fmt, vet, and test"
	@echo "  info            - Show current platform information"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "Platform-specific binaries are created in $(BUILD_DIR)/ with suffixes:"
	@echo "  -linux-amd64   - Linux (Intel/AMD 64-bit)"
	@echo "  -darwin-amd64  - macOS (Intel)"
	@echo "  -darwin-arm64  - macOS (Apple Silicon)"
	@echo ""
	@echo "The launcher script ($(BINARY_NAME).sh) automatically selects the right binary."
	@echo ""
	@echo "Docker/Cloud Run targets:"
	@echo "  docker-auth       - Configure Docker auth for Artifact Registry (one-time)"
	@echo "  docker-build      - Build container image locally (for testing)"
	@echo ""
	@echo "Terraform targets (run 'make terraform-help' for details):"
	@echo "  init-plan         - Plan initialization resources"
	@echo "  init-deploy       - Deploy initialization resources"
	@echo "  plan              - Plan main infrastructure"
	@echo "  deploy            - Deploy main infrastructure"


# ============================================
# Docker and Cloud Run Deployment
# ============================================

# GCP configuration (loaded from config.yaml or override with env vars)
GCP_PROJECT ?= $(shell grep 'project_id:' config.yaml 2>/dev/null | head -1 | awk '{print $$2}')
GCP_REGION ?= $(shell grep 'location:' config.yaml 2>/dev/null | head -1 | awk '{print $$2}')

# Configure Docker authentication for Artifact Registry (one-time setup)
# Run this once on a new machine before first deployment
docker-auth:
	@echo "Configuring Docker authentication for Artifact Registry..."
	gcloud auth configure-docker $(GCP_REGION)-docker.pkg.dev --quiet
	@echo "Docker authentication configured."
	@echo ""
	@echo "This is a one-time setup. You can now run 'make deploy'."

# Build Docker image locally (for testing only)
# NOTE: For production, use 'make deploy' which builds via Terraform
docker-build:
	@echo "Building Docker image locally for testing..."
	docker build -t speech-to-text-mcp:local .
	@echo ""
	@echo "To run locally:"
	@echo "  docker run -p 8080:8080 speech-to-text-mcp:local"


# ============================================
# Terraform Infrastructure Management
# ============================================

# Check if init has been deployed (by checking if state backend exists)
check-init:
	@if [ ! -d "init/.terraform" ]; then \
		echo ""; \
		echo "ERROR: Initialization not completed!"; \
		echo ""; \
		echo "You must run initialization BEFORE deploying main infrastructure:"; \
		echo ""; \
		echo "  1. make init-plan       # Review what will be created"; \
		echo "  2. make init-deploy     # Deploy state backend & service accounts"; \
		echo "  3. make plan            # Then plan main infrastructure"; \
		echo "  4. make deploy          # Finally deploy main infrastructure"; \
		echo ""; \
		echo "The init step creates:"; \
		echo "  - Terraform state backend (GCS bucket)"; \
		echo "  - Service accounts / IAM roles"; \
		echo "  - API enablement"; \
		echo ""; \
		exit 1; \
	fi

# Update iac/provider.tf with backend configuration from init/
update-backend:
	@echo "Updating iac/provider.tf with backend configuration..."
	@if [ ! -d "init/.terraform" ]; then \
		echo "Error: init/.terraform not found. Run 'make init-deploy' first."; \
		exit 1; \
	fi
	@if [ ! -f "iac/provider.tf.template" ]; then \
		echo "Error: iac/provider.tf.template not found."; \
		exit 1; \
	fi
	@if [ -f "iac/provider.tf" ] && grep -q 'backend "' iac/provider.tf && ! grep -q 'BACKEND_PLACEHOLDER' iac/provider.tf; then \
		echo "Warning: iac/provider.tf already has a backend configured. Skipping."; \
	else \
		BACKEND_CONFIG=$$(cd init && terraform output -raw backend_config 2>/dev/null); \
		if [ -z "$$BACKEND_CONFIG" ]; then \
			echo "Error: Could not get backend_config from terraform output."; \
			exit 1; \
		fi; \
		awk -v backend="$$BACKEND_CONFIG" ' \
			/BACKEND_PLACEHOLDER/ { \
				n = split(backend, lines, "\n"); \
				for (i = 1; i <= n; i++) { \
					gsub(/^[ \t]+|[ \t]+$$/, "", lines[i]); \
					if (lines[i] != "") print "  " lines[i]; \
				} \
				next \
			} \
			{ print } \
		' iac/provider.tf.template > iac/provider.tf; \
		echo "Successfully updated iac/provider.tf"; \
		echo ""; \
		echo "Backend configuration:"; \
		echo "$$BACKEND_CONFIG"; \
	fi

# IAC targets (main infrastructure)
plan: check-init
	@echo "Planning main infrastructure..."
	cd iac && terraform init && terraform plan

deploy: check-init
	@echo "Deploying main infrastructure..."
	cd iac && terraform init && terraform apply -auto-approve

undeploy: check-init
	@echo "Destroying main infrastructure..."
	cd iac && terraform destroy -auto-approve

# Init targets (backend, state, service accounts)
init-plan:
	@echo "Planning initialization..."
	cd init && terraform init && terraform plan

init-deploy:
	@echo "Deploying initialization..."
	cd init && terraform init && terraform apply -auto-approve
	@$(MAKE) update-backend
	@echo ""
	@echo "Initialization complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Run: make plan"
	@echo "  2. Run: make deploy"

init-destroy:
	@echo "Destroying initialization resources..."
	@echo "WARNING: This will destroy state backend and service accounts!"
	@read -p "Are you sure? (yes/no): " answer && [ "$$answer" = "yes" ]
	cd init && terraform destroy -auto-approve

# Terraform help
terraform-help:
	@echo "Terraform Makefile Targets:"
	@echo ""
	@echo "Deployment Workflow (First Time):"
	@echo "  1. make init-plan       - Plan initialization (state backend, service accounts)"
	@echo "  2. make init-deploy     - Deploy initialization"
	@echo "  3. make plan            - Plan main infrastructure"
	@echo "  4. make deploy          - Deploy main infrastructure"
	@echo ""
	@echo "Main Infrastructure:"
	@echo "  make plan               - Plan main infrastructure changes"
	@echo "  make deploy             - Deploy main infrastructure"
	@echo "  make undeploy           - Destroy main infrastructure"
	@echo ""
	@echo "Initialization (One-time Setup):"
	@echo "  make init-plan          - Plan initialization resources"
	@echo "  make init-deploy        - Deploy initialization resources"
	@echo "  make init-destroy       - Destroy initialization (DANGEROUS!)"
	@echo ""
	@echo "Utilities:"
	@echo "  make update-backend     - Update iac/provider.tf with backend config"
	@echo ""
	@echo "Note: You must run 'make init-deploy' BEFORE running 'make deploy'"
