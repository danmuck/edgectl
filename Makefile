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
	go run ./cmd/testctl -mode run -pkg ./...

test-ghost:
	go run ./cmd/testctl -mode run -pkg ./internal/ghost

test-protocol:
	go run ./cmd/testctl -mode run -pkg ./internal/protocol/...

test-seeds:
	go run ./cmd/testctl -mode run -pkg ./internal/seeds

test-seeds-live:
	GHOST_TEST_LIVE_MONGOD=1 go run ./cmd/testctl -mode run -pkg ./internal/seeds -run 'TestMongodSeedVersionCommandLive'

test-one:
	go run ./cmd/testctl -mode run -pkg '$(PKG)' -run '$(TEST)'

test-list:
	go run ./cmd/testctl -mode list -pkg '$(PKG)'

###  RUN
run-mirage:
	go run ./cmd/miragectl

run-ghost:
	go run ./cmd/ghostctl
