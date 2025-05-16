GOLANGCI_LINT_VERSION = 2.1.6
AIR_VERSION = 1.61.7

.PHONY: all build lint fix test gopls-check start generate

start: .cache/bin/air
	air --tmp_dir .cache/tmp --build.cmd "go build -o ./build/locaccel cmd/locaccel/locaccel.go" --build.bin "./build/locaccel"

all: build lint gopls-check test

build: generate
	go build -trimpath -o build/locaccel ./cmd/locaccel

lint: .cache/bin/golangci-lint generate
	go mod tidy -diff
	$< run ./...
	$< fmt --diff-colored

fix: .cache/bin/golangci-lint generate
	go mod tidy
	$< run --fix ./...
	$< fmt

test: generate
	go test -coverprofile .coverage ./...
	go tool cover -func .coverage

gopls-check:
	@res=$$(find . -name "*.go" -not -name "*_gen.go" -not -name "*_gen_test.go" -exec gopls check {} \;); echo $${res} && [[ -z $${res} ]]

generate: internal/httpclient/types_gen.go
internal/httpclient/types_gen.go: internal/httpclient/types.go
	go generate $<

.cache/bin/golangci-lint:
	@mkdir -p $(dir $@)
	curl -L https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz | tar -xzf - -C $(dir $@) --anchored --wildcards '*golangci-lint' --transform='s/.*/golangci-lint/'
	chmod +x $@

.cache/bin/air:
	@mkdir -p $(dir $@)
	curl -L https://github.com/air-verse/air/releases/download/v${AIR_VERSION}/air_${AIR_VERSION}_linux_amd64.tar.gz | tar -xzf - -C $(dir $@) -- air
	chmod +x $@
