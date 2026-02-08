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

run-pi:
	go run ./cmd/ghostctl -config cmd/ghostctl/pi.config.toml

run-client:
	@printf "Run client for ghost or mirage? [g/m] (default g): "; \
	read mode; \
	if [ -z "$$mode" ]; then mode=g; fi; \
	case "$$mode" in \
		g|ghost) mode=ghost ;; \
		m|mirage) mode=mirage ;; \
		e|exit) echo "cancelled"; exit 0 ;; \
		*) echo "invalid mode '$$mode', expected ghost or mirage"; exit 1 ;; \
	esac; \
	go run ./cmd/client-tm -mode $$mode
