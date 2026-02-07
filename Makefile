.PHONY: \
	clear \
	test \
	test-override \
	run-mirage \
	run-ghost

CLEAR_CMD ?= clear

clear:
	@$(CLEAR_CMD)

### TEST
test:
	go run ./cmd/testctl -mode interactive -pkg ./...

test-override:
	go run ./cmd/testctl -mode run -pkg ./...

###  RUN
run-mirage:
	go run ./cmd/miragectl

run-ghost:
	go run ./cmd/ghostctl
