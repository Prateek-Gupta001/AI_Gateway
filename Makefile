# The name of the binary to build
BINARY_NAME=AIGateway

# Basic build command
build:
	@echo "Building..."
	go build -o bin/$(BINARY_NAME) main.go

# Run the application
run:
	go run . 

# Run tests recursively
test:
	go test -v ./...

# Clean up binaries
clean:
	@echo "Cleaning..."
	go clean
	rm -f bin/$(BINARY_NAME)

# Declare targets that are not physical files
.PHONY: build run test clean