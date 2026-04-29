.PHONY: ci lint lint-fix test build tidy

ci: lint test build

lint:
	go tool golangci-lint run ./...

lint-fix:
	go tool golangci-lint run --fix ./...

test:
	go test ./...

build:
	go build ./...

tidy:
	go mod tidy
