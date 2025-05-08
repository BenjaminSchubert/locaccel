GOLANGCI_LINT_VERSION = 2.1.6

.PHONY: all lint fix test

all: lint test

lint: .cache/bin/golangci-lint
	go mod tidy -diff
	$< run ./...
	$< fmt --diff-colored

fix: .cache/bin/golangci-lint
	go mod tidy
	$< run --fix ./...
	$< fmt

test:
	go test -coverprofile .coverage ./...
	go tool cover -func .coverage


.cache/bin/golangci-lint:
	@mkdir -p $(dir $@)
	curl -L https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz | tar -xzf - --anchored --wildcards '*golangci-lint' --transform='s/.*/golangci-lint/'
	chmod +x $@
