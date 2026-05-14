.PHONY: ci lint lint-fix test build tidy generate verify

GO_BUILD_TAGS := containers_image_openpgp

ci: lint verify test build

lint:
	go tool golangci-lint run --build-tags "$(GO_BUILD_TAGS)" ./...

lint-fix:
	go tool golangci-lint run --build-tags "$(GO_BUILD_TAGS)" --fix ./...

test:
	go test -tags "$(GO_BUILD_TAGS)" ./...

build:
	go build -tags "$(GO_BUILD_TAGS)" ./...

tidy:
	go mod tidy

generate:
	go generate -tags "$(GO_BUILD_TAGS)" ./...

verify:
	./hack/diff.sh generate
