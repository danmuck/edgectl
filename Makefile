.PHONY: test run-mirage run-ghost

test:
	go clean -testcache
	go test -v ./...

run-mirage:
	go run ./cmd/miragectl

run-ghost:
	go run ./cmd/ghostctl
