.PHONY: build test run clean install

# Build the binary
build:
	go build -o bin/goarctis cmd/goarctis/main.go

# Run tests
test:
	go test ./... -cover

# Run tests with coverage report
test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

# Run the application
run:
	go run cmd/goarctis/main.go

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out

# Install dependencies
deps:
	go mod download
	go mod tidy

# Build for Linux
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/goarctis-linux-amd64 cmd/goarctis/main.go

# Install to /usr/local/bin
.PHONY: update-systemd
update-systemd:
	@./update_systemd.sh

.DEFAULT_GOAL := build