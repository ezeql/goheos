.PHONY: fmt lint test race cover vuln tidy

fmt:
	golangci-lint fmt

lint:
	golangci-lint run

test:
	go test ./...

race:
	go test -race -count=1 ./...

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tidy:
	go mod tidy
	git diff --exit-code go.mod
