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
	build-all \
	update-pi

include .env

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

certs:
	@bash -lc 'mkdir -p certs && \
		openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes -keyout certs/ca.key -out certs/ca.crt -subj "/CN=edgectl-ca" && \
		openssl req -newkey rsa:2048 -nodes -keyout certs/mirage.mac.key -out certs/mirage.mac.csr -subj "/CN=mirage.mac" && printf "subjectAltName=DNS:mirage.mac,IP:192.168.50.94\nextendedKeyUsage=serverAuth\n" > certs/mirage.mac.ext && openssl x509 -req -in certs/mirage.mac.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/mirage.mac.crt -days 825 -sha256 -extfile certs/mirage.mac.ext && \
		openssl req -newkey rsa:2048 -nodes -keyout certs/ghost.pi.key -out certs/ghost.pi.csr -subj "/CN=ghost.pi" && printf "subjectAltName=DNS:ghost.pi\nextendedKeyUsage=clientAuth\n" > certs/ghost.pi.ext && openssl x509 -req -in certs/ghost.pi.csr -CA certs/ca.crt -CAkey certs/ca.key -CAserial certs/ca.srl -out certs/ghost.pi.crt -days 825 -sha256 -extfile certs/ghost.pi.ext
		'


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
	rsync -avz --delete \
	  --filter='P cmd/ghostctl/*.tls.config.toml' \
	  --filter='P cmd/ghostctl/*.config.toml' \
	  cmd internal pkg certs .air.toml .env Makefile \
	  "${PI_USER}@${PI_HOST}:${PI_PATH}/"
