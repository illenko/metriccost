.PHONY: build run test lint clean build-all

BINARY_NAME=whodidthis
VERSION?=0.1.0
BUILD_DIR=bin
MAIN_PATH=./cmd/whodidthis

# Build for current platform
build:
	go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

# Run the application
run:
	go run $(MAIN_PATH)

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Install dependencies
deps:
	go mod download
	go mod tidy

# Build for all platforms
build-all: clean
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

# Development: run with hot reload (requires air)
dev:
	air

# Build and run
build-run: build
	./$(BUILD_DIR)/$(BINARY_NAME)
