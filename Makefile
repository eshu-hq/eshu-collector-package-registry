# Local parity for the CI lanes: `make ci` runs the lint, test, and race
# commands the workflow runs. Coverage artifacts and SARIF upload stay in
# CI; use `go tool cover` locally for the same numbers.
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

.PHONY: ci vet lint fmt tidy cap build test race

ci: vet lint fmt tidy cap build test race

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	test -z "$$($(GOLANGCI_LINT) fmt --diff ./... 2>/dev/null)"

tidy:
	go mod tidy -diff

cap:
	git ls-files '*.go' | xargs wc -l | grep -v ' total$$' | awk '$$1 > 500 {bad=1; print} END {exit bad}'

build:
	go build ./...

test:
	go test ./... -count=1

race:
	go test ./... -race -count=1
