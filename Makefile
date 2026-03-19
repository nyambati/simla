PHONY: generate test

generate:
	go generate ./...

test: generate build_test
	go test ./... -v

ptest: generate build_test
	@gotestsum --format=testname

build_test:
	GOOS=linux go build -o bin/main examples/go/main.go
