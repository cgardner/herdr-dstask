BINARY  := herdr-dstask
VERSION := $(shell sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml | head -1)
LDFLAGS := -s -w -X github.com/cgardner/herdr-dstask/internal/cli.version=$(VERSION)

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the plugin binary into bin/
	@mkdir -p bin
	@go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) .
	@echo "built bin/$(BINARY) $(VERSION)"

.PHONY: test
test: ## Run the test suite (it never touches ~/.dstask)
	@go test ./...

.PHONY: lint
lint: ## Fail if anything is unformatted or vet reports a problem
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	@go vet ./...

.PHONY: fmt
fmt: ## Rewrite sources with gofmt
	@gofmt -w .

.PHONY: ci
ci: lint test ## Everything CI would run

.PHONY: link
link: build ## Link this working copy into the running Herdr session
	@herdr plugin link "$(CURDIR)" >/dev/null
	@herdr plugin list --plugin cgardner.$(BINARY) --json >/dev/null && echo "linked $(CURDIR)"

.PHONY: unlink
unlink: ## Remove the linked plugin from the running Herdr session
	@herdr plugin unlink cgardner.$(BINARY) >/dev/null && echo "unlinked"

.PHONY: run
run: build ## Open the UI in this terminal, outside Herdr
	@./bin/$(BINARY)

.PHONY: list
list: build ## Print the open tasks without the UI
	@./bin/$(BINARY) --list

.PHONY: sandbox
sandbox: build ## Open the popup on a copy of ~/.dstask (FRESH=1 copies again)
	@bash scripts/sandbox.sh $(if $(FRESH),--fresh)

.PHONY: clean
clean: ## Remove build output
	@rm -rf bin

.PHONY: help
help: ## List the targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  %-8s %s\n", $$1, $$2}'
