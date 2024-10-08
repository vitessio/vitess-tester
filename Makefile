.PHONY: all build test tidy clean

GO := go
REQUIRED_GO_VERSION := 1.23

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