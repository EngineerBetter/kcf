.PHONY: all format lint vet build test tools

all: format lint build test vet

format:
	make/format

lint:
	make/lint

vet:
	make/vet

tools:
	make/tools

test:
	go test -race -cover $$(go list -f '{{ .ImportPath }}' ./... | grep -v vendor)

build:
	go build
