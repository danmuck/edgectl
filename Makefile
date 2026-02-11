.PHONY: \
	clear \
	test \
	test-override \
	run-client \
	run-mirage \
	run-ghost \
	build-ghost \
	build-mirage \
	build-client \
	build-all

CLEAR_CMD ?= clear

clear:
	@$(CLEAR_CMD)

### TEST
test:
	clear; go run ./cmd/testctl -mode interactive -pkg ./...

test-override:
	clear; go run ./cmd/testctl -mode run -pkg ./...

###  RUN
run-mirage:
	clear; go run ./cmd/miragectl

run-ghost:
	clear; go run ./cmd/ghostctl

run-pi:
	clear; go run ./cmd/ghostctl -config cmd/ghostctl/pi.tls.config.toml

run-tls:
	clear; go run ./cmd/miragectl -config cmd/miragectl/mac.tls.config.toml


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

### BUILD
build-ghost:
	@mkdir -p local/bin
	go build -o local/bin/ghostctl ./cmd/ghostctl

build-mirage:
	@mkdir -p local/bin
	go build -o local/bin/miragectl ./cmd/miragectl

build-client:
	@mkdir -p local/bin
	go build -o local/bin/client-tm ./cmd/client-tm

build-all: build-ghost build-mirage build-client
