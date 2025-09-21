# Justfile for env-lease

# Default task to run when no command is specified
default: lint test

# Build the application
build:
    go build ./...

# Run linter
lint:
    @echo "Running linter..."
    @go vet ./...
    # For more comprehensive linting, install and use golangci-lint
    # @golangci-lint run

# Run all tests
test:
    @echo "Running tests..."
    @go test ./...

# Run a specific test by name, e.g., `just test-one TestMyFunction`
test-one name:
    @echo "Running test: {{name}}"
    @go test . -run '^{{name}}$'

# Format the code
fmt:
    @echo "Formatting code..."
    @gofmt -w .
