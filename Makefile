.PHONY: \
	clear \
	test \
	test-override \
	run-client \
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

run-client:
	@printf "Run client for ghost or mirage? [ghost/mirage] (default ghost): "; \
	read mode; \
	if [ -z "$$mode" ]; then mode=ghost; fi; \
	case "$$mode" in \
		ghost|mirage) ;; \
		*) echo "invalid mode '$$mode', expected ghost or mirage"; exit 1 ;; \
	esac; \
	go run ./cmd/client-tm -mode $$mode
