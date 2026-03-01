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
	gotestsum -- $$(go list ./... | grep -v '/cmd/') -cover

# Run tests in CI with coverage output for Coveralls (exclude cmd directories)
test-ci:
	gotestsum -- $$(go list ./... | grep -v '/cmd/') -coverprofile=coverage.out

# Run tests with coverage report (exclude cmd directories)
test-coverage:
	gotestsum -- $$(go list ./... | grep -v '/cmd/') -coverprofile=coverage.out
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
# example: make release VERSION=v0.2.0
release:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then \
		echo "Error: VERSION must be set (e.g., VERSION=v0.2.0)"; \
		exit 1; \
	fi
	@if ! echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "Error: VERSION must be in semantic version format (e.g., v0.2.0)"; \
		exit 1; \
	fi
	@echo "Checking git status..."
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory is not clean. Commit or stash changes first."; \
		exit 1; \
	fi
	@echo "Checking if tag $(VERSION) already exists..."
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "Error: Tag $(VERSION) already exists"; \
		exit 1; \
	fi
	@echo "Checking if we're on main branch..."
	@if [ "$$(git rev-parse --abbrev-ref HEAD)" != "main" ]; then \
		echo "Warning: Not on main branch. Continuing anyway..."; \
	fi
	@echo "Creating release tag $(VERSION)..."
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@echo "Pushing tag $(VERSION) to remote..."
	@git push origin $(VERSION)
	@echo "Release $(VERSION) created and pushed successfully!"
	@echo "GitHub Actions will automatically create a release with the binary."

# Test Razer discovery only
test-razer:
	go run cmd/test-razer/main.go

# Install to /usr/local/bin
.PHONY: update-systemd
update-systemd:
	@./scripts/update_systemd.sh

.DEFAULT_GOAL := build
