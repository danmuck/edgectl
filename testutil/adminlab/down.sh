#!/bin/sh
# Stops the lab processes started by up.sh.
cd "$(dirname "$0")/../.."
for f in local/adminlab/mirage.pid local/adminlab/ghost.pid; do
  [ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null; rm -f "$f"
done
echo "lab down"
