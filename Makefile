PHONY: generate test

generate:
	go generate ./...

test: generate build_test
	go test ./... -v

ptest: generate build_test
	@gotestsum --format=testname

build_test:
	GOOS=linux GOARCH=amd64 go build -o bin/main test/main.go
