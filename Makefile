.PHONY: \
	clear \
	cls \
	test \
	test-clear \
	test-ghost \
	test-protocol \
	test-seeds \
	test-one \
	test-one-clear \
	test-list \
	run-mirage \
	run-ghost \
	run-mirage-clear \
	run-ghost-clear

PKG ?= ./internal/ghost
TEST ?= .

clear:
	@clear || printf '\033[2J\033[H'

cls: clear

### TEST
test:
	go clean -testcache
	go test -v ./...

test-clear: clear test

test-ghost:
	go test -v ./internal/ghost

test-protocol:
	go test -v ./internal/protocol/...

test-seeds:
	go test -v ./internal/seeds

test-one:
	go test -v $(PKG) -run '$(TEST)'

test-one-clear: clear test-one

test-list:
	go test $(PKG) -list .

###  RUN
run-mirage:
	go run ./cmd/miragectl

run-ghost:
	go run ./cmd/ghostctl

run-mirage-clear: clear run-mirage

run-ghost-clear: clear run-ghost
