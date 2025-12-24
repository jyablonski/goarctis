.PHONY: build test run clean install release

# Version detection
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
VERSION_PKG := github.com/jyablonski/goarctis/pkg/version
LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION)

# Build the binary
build:
	go build -ldflags "$(LDFLAGS)" -o bin/goarctis cmd/goarctis/main.go

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
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/goarctis-linux-amd64 cmd/goarctis/main.go

# Build test version (different binary name to avoid conflicts)
# run this when testing locally on your machine w/ code changes
build-test:
	go build -ldflags "$(LDFLAGS)" -o bin/goarctis-test cmd/goarctis/main.go

# Release: create and push a git tag
# Usage: make release VERSION=v0.2.0
release:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then \
		echo "Error: VERSION must be set (e.g., VERSION=v0.2.0)"; \
		exit 1; \
	fi
	@echo "Creating release tag $(VERSION)..."
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@echo "Pushing tag $(VERSION) to remote..."
	@git push origin $(VERSION)
	@echo "Release $(VERSION) created and pushed successfully!"

# Test Razer discovery only
test-razer:
	go run cmd/test-razer/main.go

# Install to /usr/local/bin
.PHONY: update-systemd
update-systemd:
	@./scripts/update_systemd.sh

.DEFAULT_GOAL := build
