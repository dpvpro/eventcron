# Makefile for eventcrone project

# Project configuration
PROJECT_NAME = eventcrone
VERSION = $(shell cat VERSION 2>/dev/null || echo "1.0.0")
GOVERSION = $(shell go version | awk '{print $$3}')

# Build configuration
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

# Installation paths
PREFIX ?= /usr/local
BINDIR = $(PREFIX)/bin
SBINDIR = $(PREFIX)/sbin
SYSCONFDIR = /etc
MANDIR = $(PREFIX)/share/man
DOCDIR = $(PREFIX)/share/doc/$(PROJECT_NAME)
USERTABLEDIR = /var/spool/eventcrone
SYSTEMTABLEDIR = /etc/eventcrone.d

# Build flags
LDFLAGS = -s -w -X main.version=$(VERSION) -X main.buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_FLAGS = -ldflags "$(LDFLAGS)" -trimpath

# Go tools
GO = go
GOFMT = gofmt
GOLINT = golangci-lint

# Targets
DAEMON = eventcroned
CLIENT = eventcronetab

# Default target
.PHONY: all
all: build

# Build targets
.PHONY: build
build: $(DAEMON) $(CLIENT)

$(DAEMON):
	@echo "Building $(DAEMON)..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build $(BUILD_FLAGS) -o $(DAEMON) ./cmd/$(DAEMON)

$(CLIENT):
	@echo "Building $(CLIENT)..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build $(BUILD_FLAGS) -o $(CLIENT) ./cmd/$(CLIENT)

# Development targets
.PHONY: dev
dev:
	@echo "Building development version..."
	$(GO) build -race -o $(DAEMON) ./cmd/$(DAEMON)
	$(GO) build -race -o $(CLIENT) ./cmd/$(CLIENT)

.PHONY: debug
debug:
	@echo "Building debug version..."
	$(GO) build -gcflags "all=-N -l" -o $(DAEMON) ./cmd/$(DAEMON)
	$(GO) build -gcflags "all=-N -l" -o $(CLIENT) ./cmd/$(CLIENT)

# Testing
.PHONY: test
test:
	@echo "Running tests..."
	$(GO) test -v ./...

.PHONY: test-race
test-race:
	@echo "Running tests with race detector..."
	$(GO) test -race -v ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Code quality
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

.PHONY: lint
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

.PHONY: vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

.PHONY: check
check: fmt vet lint test

# Dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download

.PHONY: deps-update
deps-update:
	@echo "Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy

.PHONY: deps-verify
deps-verify:
	@echo "Verifying dependencies..."
	$(GO) mod verify

# Installation
.PHONY: install
install: build install-dirs install-bins install-config

.PHONY: install-dirs
install-dirs:
	@echo "Creating installation directories..."
	install -d $(DESTDIR)$(BINDIR)
	install -d $(DESTDIR)$(SBINDIR)
	install -d $(DESTDIR)$(SYSCONFDIR)
	install -d $(DESTDIR)$(DOCDIR)
	install -d $(DESTDIR)$(USERTABLEDIR)
	install -d $(DESTDIR)$(SYSTEMTABLEDIR)

.PHONY: install-bins
install-bins:
	@echo "Installing binaries..."
	install -m 0755 $(DAEMON) $(DESTDIR)$(SBINDIR)/
	install -m 4755 $(CLIENT) $(DESTDIR)$(BINDIR)/

.PHONY: install-config
install-config:
	@echo "Installing configuration files..."
	@if [ ! -f $(DESTDIR)$(SYSCONFDIR)/eventcrone.conf ]; then \
		echo "# eventcrone configuration file" > $(DESTDIR)$(SYSCONFDIR)/eventcrone.conf; \
		echo "# This file is currently unused but reserved for future configuration options" >> $(DESTDIR)$(SYSCONFDIR)/eventcrone.conf; \
	fi

# Man pages (placeholder - to be implemented)
.PHONY: install-man
install-man:
	@echo "Man pages not yet implemented"

# Uninstallation
.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(PROJECT_NAME)..."
	rm -f $(DESTDIR)$(SBINDIR)/$(DAEMON)
	rm -f $(DESTDIR)$(BINDIR)/$(CLIENT)
	rm -rf $(DESTDIR)$(DOCDIR)

# Packaging
.PHONY: package
package: clean build
	@echo "Creating package..."
	mkdir -p dist/$(PROJECT_NAME)-$(VERSION)
	cp $(DAEMON) $(CLIENT) dist/$(PROJECT_NAME)-$(VERSION)/
	cp README.md LICENSE dist/$(PROJECT_NAME)-$(VERSION)/
	cd dist && tar -czf $(PROJECT_NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz $(PROJECT_NAME)-$(VERSION)
	@echo "Package created: dist/$(PROJECT_NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz"

# Cross-compilation
.PHONY: build-all
build-all: clean
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 $(MAKE) build && mv $(DAEMON) $(DAEMON)-linux-amd64 && mv $(CLIENT) $(CLIENT)-linux-amd64
	GOOS=linux GOARCH=arm64 $(MAKE) build && mv $(DAEMON) $(DAEMON)-linux-arm64 && mv $(CLIENT) $(CLIENT)-linux-arm64
	GOOS=linux GOARCH=386 $(MAKE) build && mv $(DAEMON) $(DAEMON)-linux-386 && mv $(CLIENT) $(CLIENT)-linux-386

# Docker targets
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(PROJECT_NAME):$(VERSION) .
	docker tag $(PROJECT_NAME):$(VERSION) $(PROJECT_NAME):latest

.PHONY: docker-run
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it --privileged -v /tmp:/tmp $(PROJECT_NAME):$(VERSION)

# Systemd service
.PHONY: install-systemd
install-systemd:
	@echo "Installing systemd service..."
	@echo "[Unit]" > $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "Description=Inotify cron daemon" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "After=network.target" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "[Service]" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "Type=forking" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "ExecStart=$(SBINDIR)/$(DAEMON)" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "ExecReload=/bin/kill -HUP \$$MAINPID" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "PIDFile=/run/eventcroned.pid" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "Restart=on-failure" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "[Install]" >> $(DESTDIR)/etc/systemd/system/eventcroned.service
	@echo "WantedBy=multi-user.target" >> $(DESTDIR)/etc/systemd/system/eventcroned.service

# Cleanup
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -f $(DAEMON) $(CLIENT)
	rm -f $(DAEMON)-* $(CLIENT)-*
	rm -f coverage.out coverage.html
	rm -rf dist/

.PHONY: distclean
distclean: clean
	@echo "Deep cleaning..."
	$(GO) clean -cache -testcache -modcache

# Development utilities
.PHONY: run-daemon
run-daemon: $(DAEMON)
	@echo "Running daemon in foreground..."
	sudo ./$(DAEMON) -n

.PHONY: run-client
run-client: $(CLIENT)
	@echo "Running client..."
	./$(CLIENT) -l

# Information
.PHONY: info
info:
	@echo "Project: $(PROJECT_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Go version: $(GOVERSION)"
	@echo "Target OS: $(GOOS)"
	@echo "Target Arch: $(GOARCH)"
	@echo "CGO Enabled: $(CGO_ENABLED)"

.PHONY: version
version:
	@echo $(VERSION)

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build       - Build binaries"
	@echo "  dev         - Build development version with race detector"
	@echo "  debug       - Build debug version"
	@echo "  test        - Run tests"
	@echo "  test-race   - Run tests with race detector"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  fmt         - Format code"
	@echo "  lint        - Run linter"
	@echo "  vet         - Run go vet"
	@echo "  check       - Run all code quality checks"
	@echo "  deps        - Download dependencies"
	@echo "  deps-update - Update dependencies"
	@echo "  install     - Install binaries and config files"
	@echo "  uninstall   - Uninstall binaries"
	@echo "  package     - Create distribution package"
	@echo "  build-all   - Cross-compile for multiple platforms"
	@echo "  clean       - Clean build artifacts"
	@echo "  distclean   - Deep clean including caches"
	@echo "  info        - Show build information"
	@echo "  help        - Show this help"

# Ensure proper dependencies are installed
.PHONY: setup-dev
setup-dev:
	@echo "Setting up development environment..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Default target when no arguments provided
.DEFAULT_GOAL := help