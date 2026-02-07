.PHONY: \
	clear \
	test \
	test-ghost \
	test-protocol \
	test-seeds \
	test-seeds-live \
	test-one \
	test-list \
	run-mirage \
	run-ghost

PKG ?= ./internal/ghost
TEST ?= .
CLEAR_CMD ?= clear

clear:
	@$(CLEAR_CMD)

### TEST
test:
	go clean -testcache
	go test -v ./...

test-ghost:
	go test -v ./internal/ghost

test-protocol:
	go test -v ./internal/protocol/...

test-seeds:
	go test -v ./internal/seeds

test-seeds-live:
	GHOST_TEST_LIVE_MONGOD=1 go test -v ./internal/seeds -run 'TestMongodSeedVersionCommandLive'

test-one:
	go test -v $(PKG) -run '$(TEST)'

test-list:
	go test $(PKG) -list .

###  RUN
run-mirage:
	go run ./cmd/miragectl

run-ghost:
	go run ./cmd/ghostctl
