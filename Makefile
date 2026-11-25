.PHONY: \
	clear \
	test \
	test-override \
	run-client \
	lab \
	run-mirage \
	run-ghost \
	build-ghost \
	build-mirage \
	build-client \
	build-all \
	update-pi \
	certs \
	certs-root \
	certs-operator \
	certs-mirage \
	certs-ghost

# Optional: hosts that receive this Makefile via update-pi have no .env.
-include .env

CLEAR_CMD ?= clear

clear:
	@$(CLEAR_CMD)

### TEST
test:
	clear; go run ./cmd/testctl -mode interactive -pkg ./...

test-override:
	clear; go run ./cmd/testctl -mode run -pkg ./...

###  RUN
# One-command local stack: throwaway certificates, a Mirage and a Ghost on loopback, and a menu
# to add Ghosts and run commands. Everything lives in local/lab (docs/progress/lab_mode.md).
lab:
	@go run ./cmd/client-tm -mode lab

run-mirage:
	clear; go run ./cmd/miragectl

run-ghost:
	clear; go run ./cmd/ghostctl

run-pi:
	clear; go run ./cmd/ghostctl -config cmd/ghostctl/pi.tls.config.toml

run-tls:
	clear; go run ./cmd/miragectl -config cmd/miragectl/mac.tls.config.toml

# Certificates (docs/architecture/definitions/admin_security.toml).
# Principals are edgectl://<role>/<id> in one URI SAN; ECDSA P-256; leaves last at most 90 days.
# certs/root holds the root CA key: keep it offline, never copy it to Mirage or Ghost hosts.
# Runtimes need only certs/ca.crt plus their own <id>.crt and <id>.key (0600).
# Existing files are never overwritten; remove a stale cert/key pair before re-issuing.
certs:
	@echo "usage:"
	@echo "  make certs-root                              # once; offline root CA in certs/root"
	@echo "  make certs-operator NAME=<id>                # operator client cert"
	@echo "  make certs-mirage ID=<id> [DNS=a,b] [IP=x]   # Mirage cert (add its host name/IP)"
	@echo "  make certs-ghost ID=<ghost_id> [DNS=] [IP=]  # Ghost cert; ID must equal ghost_id"

certs-root:
	go run ./cmd/edgecert root -out certs/root
	cp certs/root/ca.crt certs/ca.crt

certs-operator:
	@test -n "$(NAME)" || { echo "usage: make certs-operator NAME=<id>"; exit 1; }
	go run ./cmd/edgecert issue -role operator -id "$(NAME)" -out certs

certs-mirage:
	@test -n "$(ID)" || { echo "usage: make certs-mirage ID=<id> [DNS=a,b] [IP=x,y]"; exit 1; }
	go run ./cmd/edgecert issue -role mirage -id "$(ID)" -dns "$(DNS)" -ip "$(IP)" -out certs

certs-ghost:
	@test -n "$(ID)" || { echo "usage: make certs-ghost ID=<ghost_id> [DNS=a,b] [IP=x,y]"; exit 1; }
	go run ./cmd/edgecert issue -role ghost -id "$(ID)" -dns "$(DNS)" -ip "$(IP)" -out certs


run-client:
	@printf "Run client for mirage or ghost? [m/g] (default m): "; \
	read mode; \
	if [ -z "$$mode" ]; then mode=m; fi; \
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

### DEPLOY
update-pi:
	# rsync -avz --delete cmd internal pkg .air.toml .env Makefile $(PI_USER)@$(PI_HOST):$(PI_PATH)/
	# The root CA key and .env stay on this host (see the certs note above): runtimes need only
	# certs/ca.crt plus their own cert and key. Copies already on the target are not removed.
	rsync -avz --delete \
	  --filter='P cmd/ghostctl/*.tls.config.toml' \
	  --filter='P cmd/ghostctl/*.config.toml' \
	  --exclude='/certs/ca.key' \
	  --exclude='/certs/ca.srl' \
	  --exclude='/certs/root/' \
	  --exclude='/certs/mirage.mac.key' \
	  cmd internal pkg certs .air.toml Makefile \
	  "${PI_USER}@${PI_HOST}:${PI_PATH}/"
