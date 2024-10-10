.PHONY: all build test tidy clean pretty install-tools lint install-hooks

GO := go
REQUIRED_GO_VERSION := 1.23
GOLANGCI_LINT_VERSION := v1.55.2

# Version check
check_version:
	@GO_VERSION=$$($(GO) version | awk '{print $$3}' | sed 's/go//'); \
	MAJOR_VERSION=$$(echo $$GO_VERSION | cut -d. -f1); \
	MINOR_VERSION=$$(echo $$GO_VERSION | cut -d. -f2); \
	if [ "$$MAJOR_VERSION" -eq 1 ] && [ "$$MINOR_VERSION" -lt 23 ]; then \
		echo "Error: Go version $(REQUIRED_GO_VERSION) or higher is required. Current version is $$GO_VERSION"; \
		exit 1; \
	else \
		echo "Go version is acceptable: $$GO_VERSION"; \
	fi

default: check_version build

build:
	$(GO) build -o vt ./go/vt

test:
	$(GO) test ./go/...

tidy:
	$(GO) mod tidy

clean:
	$(GO) clean -i ./...
	rm -f vt

# Pretty: formats the code using gofumpt and goimports-reviser
pretty: install-tools
	@echo "Running formatting tools..."
	@gofumpt -l -w . >/dev/null 2>&1 || true
	@goimports-reviser -project-name $$(go list -m) -rm-unused -set-alias -format . >/dev/null 2>&1 || true

# Install tools: Checks if the required tools are installed, installs if missing
install-tools:
	@command -v gofumpt >/dev/null 2>&1 || { \
		echo "Installing gofumpt..."; \
		go install mvdan.cc/gofumpt@latest >/dev/null 2>&1; \
	}
	@command -v goimports-reviser >/dev/null 2>&1 || { \
		echo "Installing goimports-reviser..."; \
		go install github.com/incu6us/goimports-reviser@latest >/dev/null 2>&1; \
	}
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION); \
	}

# Lint: runs golangci-lint
lint: install-tools
	@echo "Running golangci-lint..."
	@golangci-lint run --config .golangci.yml ./go/...

install-hooks:
	@echo "Installing Git hooks..."
	@cp git-hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed successfully."