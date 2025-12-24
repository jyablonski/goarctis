.PHONY: build test run clean install

# Build the binary
build:
	go build -o bin/goarctis cmd/goarctis/main.go

# Run tests (exclude cmd directories to avoid covdata tool error in Go 1.25)
test:
	go test $$(go list ./... | grep -v '/cmd/') -cover

# Run tests with coverage report (exclude cmd directories)
test-coverage:
	go test $$(go list ./... | grep -v '/cmd/') -coverprofile=coverage.out
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

# Build test version (different binary name to avoid conflicts)
# run this when testing locally on your machine w/ code changes
build-test:
	go build -o bin/goarctis-test cmd/goarctis/main.go

# Test Razer discovery only
test-razer:
	go run cmd/test-razer/main.go

# Install to /usr/local/bin
.PHONY: update-systemd
update-systemd:
	@./scripts/update_systemd.sh

.DEFAULT_GOAL := build
