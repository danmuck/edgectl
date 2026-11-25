#!/bin/sh
# Starts the lab Mirage (admin 127.0.0.1:7020, managed ghost.local admin 127.0.0.1:7010, session
# 127.0.0.1:9000) and ghost.lab (admin 127.0.0.1:7011). Logs: local/adminlab/*.log.
set -eu
cd "$(dirname "$0")/../.."
./local/adminlab/miragectl -config testutil/adminlab/mirage.toml > local/adminlab/mirage.log 2>&1 &
echo $! > local/adminlab/mirage.pid
./local/adminlab/ghostctl -config testutil/adminlab/ghost.toml > local/adminlab/ghost.log 2>&1 &
echo $! > local/adminlab/ghost.pid
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  nc -z 127.0.0.1 7020 2>/dev/null && nc -z 127.0.0.1 7011 2>/dev/null && { echo "lab up"; exit 0; }
  sleep 1
done
echo "lab did not start; see local/adminlab/*.log" >&2
exit 1
