.PHONY: \
	clear \
	test \
	test-override \
	run-client \
	lab \
	serve \
	run-mirage \
	run-ghost \
	build-ghost \
	build-mirage \
	build-client \
	build-edgecert \
	gui-env \
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
# One-command local stack: lab certificates (kept between starts), a Mirage and a Ghost on
# loopback, and a menu to add Ghosts and run commands. Everything lives in local/lab
# (docs/progress/lab_mode.md).
lab:
	@go run ./cmd/client-tm -mode lab

# Same stack as `lab` without the menu: Mirage's output streams to the terminal, like a dev server.
# Drive it over api/v1. The client is built and run directly because `go run` exits 1 on Ctrl-C.
serve:
	@mkdir -p local/lab/bin
	@go build -o local/lab/bin/client-tm ./cmd/client-tm
	@local/lab/bin/client-tm -mode serve

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

# Operator tooling a web front end runs to issue Ghost certificates and sign dependency installs
# (docs/architecture/definitions/api_v1.toml [parity.signing], [parity.certificates]).
build-edgecert:
	@mkdir -p local/bin
	@go build -o local/bin/edgecert ./cmd/edgecert

build-all: build-ghost build-mirage build-client build-edgecert

# Builds edgecert and prints the settings a front end's server needs to drive the lab over api/v1,
# as KEY=value lines with absolute paths. Start the lab once first (`make lab` or `make serve`).
# These settings include the lab root CA key and both operator keys: local use only.
#   make gui-env >> ../tracker_suite/.env.local
LAB_DIR ?= local/lab
gui-env: build-edgecert
	@test -f "$(LAB_DIR)/mirage.toml" && test -f "$(LAB_DIR)/certs/root/ca.key" || \
		{ echo "no lab in $(LAB_DIR): run 'make lab' or 'make serve' once first" >&2; exit 1; }
	@addr=$$(sed -n 's/^api_v1_addr *= *"\(.*\)"/\1/p' "$(LAB_DIR)/mirage.toml"); \
	test -n "$$addr" || { echo "$(LAB_DIR)/mirage.toml has no api_v1_addr" >&2; exit 1; }; \
	certs="$(abspath $(LAB_DIR))/certs"; \
	echo "EDGECTL_API_URL=https://$$addr/api/v1"; \
	echo "EDGECTL_CLIENT_CERT=$$certs/ops.crt"; \
	echo "EDGECTL_CLIENT_KEY=$$certs/ops.key"; \
	echo "EDGECTL_CA=$$certs/ca.crt"; \
	echo "EDGECTL_EDGECERT_BIN=$(abspath local/bin/edgecert)"; \
	echo "EDGECTL_CA_ISSUER_CERT=$$certs/root/ca.crt"; \
	echo "EDGECTL_CA_ISSUER_KEY=$$certs/root/ca.key"; \
	echo "EDGECTL_CERTS_DIR=$$certs"; \
	echo "EDGECTL_APPROVER_CERT=$$certs/lead.crt"; \
	echo "EDGECTL_APPROVER_KEY=$$certs/lead.key"

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
