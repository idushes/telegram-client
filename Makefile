# Variables
BINARY_NAME=telegram-client
VERSION=1.0.0
BUILD_DIR=build
MAIN_FILE=main.go

# Platforms
PLATFORMS=linux darwin windows
ARCHITECTURES=amd64 arm64

# Go commands
GOBUILD=go build
GOCLEAN=go clean
GOTEST=go test
GOGET=go get
GOMOD=go mod

# Default target
.PHONY: all
all: clean build

# Build the project for the current platform
.PHONY: build
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Cross compile for all supported platforms
.PHONY: build-all
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@$(foreach PLATFORM,$(PLATFORMS),\
		$(foreach ARCH,$(ARCHITECTURES),\
			echo "Building for $(PLATFORM)/$(ARCH)..." && \
			GOOS=$(PLATFORM) GOARCH=$(ARCH) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-$(PLATFORM)-$(ARCH)$(if $(findstring windows,$(PLATFORM)),.exe,) $(MAIN_FILE); \
		) \
	)
	@echo "All builds complete"

# Build for Linux
.PHONY: build-linux
build-linux: clean
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	@$(foreach ARCH,$(ARCHITECTURES),\
		echo "Building for linux/$(ARCH)..." && \
		GOOS=linux GOARCH=$(ARCH) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-linux-$(ARCH) $(MAIN_FILE); \
	)
	@echo "Linux builds complete"

# Build for macOS
.PHONY: build-darwin
build-darwin: clean
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	@$(foreach ARCH,$(ARCHITECTURES),\
		echo "Building for darwin/$(ARCH)..." && \
		GOOS=darwin GOARCH=$(ARCH) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-$(ARCH) $(MAIN_FILE); \
	)
	@echo "macOS builds complete"

# Build for Windows
.PHONY: build-windows
build-windows: clean
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	@$(foreach ARCH,$(ARCHITECTURES),\
		echo "Building for windows/$(ARCH)..." && \
		GOOS=windows GOARCH=$(ARCH) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-windows-$(ARCH).exe $(MAIN_FILE); \
	)
	@echo "Windows builds complete"

# Test the project
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	$(GOCLEAN)

# Update dependencies
.PHONY: deps
deps:
	$(GOGET) -u ./...
	$(GOMOD) tidy

# Run the project
.PHONY: run
run: build
	@echo "Running application..."
	@$(BUILD_DIR)/$(BINARY_NAME)

# Help information
.PHONY: help
help:
	@echo "Make targets:"
	@echo "  build         - Build for current platform"
	@echo "  build-all     - Build for all platforms"
	@echo "  build-linux   - Build for Linux (amd64, arm64)"
	@echo "  build-darwin  - Build for macOS (amd64, arm64)"
	@echo "  build-windows - Build for Windows (amd64, arm64)"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  deps          - Update dependencies"
	@echo "  run           - Build and run the application"
	@echo "  help          - Show this help message" 