#!/bin/sh
# Admin lab setup (testutil/adminlab/README.md): run from the repo root. Creates a throwaway root CA
# and lab certificates under certs/ (gitignored; existing files are kept) and builds the runtimes
# into local/adminlab/. Never point this at a real deployment's certs/root.
set -eu
[ -f go.mod ] || { echo "run from the repo root" >&2; exit 1; }
[ -f .env ] || touch .env   # Makefile includes .env, which is gitignored
# Refuse to touch a real PKI: certs/ must be absent, empty, or created by this script (marker).
if [ -d certs ] && [ ! -f certs/.adminlab ] && [ -n "$(ls -A certs)" ]; then
  echo "certs/ already holds files not created by the lab (a real PKI?); use a fresh clone" >&2
  exit 1
fi
mkdir -p local/adminlab certs
touch certs/.adminlab
issue() { [ -f "certs/$2.crt" ] || go run ./cmd/edgecert issue -role "$1" -id "$2" -ip 127.0.0.1 -out certs; }
[ -f certs/root/ca.key ] || { go run ./cmd/edgecert root -out certs/root; cp certs/root/ca.crt certs/ca.crt; }
issue mirage mirage.lab
issue ghost ghost.local
issue ghost ghost.lab
issue operator ops
issue operator lead
go build -o local/adminlab/miragectl ./cmd/miragectl
go build -o local/adminlab/ghostctl ./cmd/ghostctl
go build -o local/adminlab/sign ./testutil/adminlab/sign
echo "ready: testutil/adminlab/up.sh"
