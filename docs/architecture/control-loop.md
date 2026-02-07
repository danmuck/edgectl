# Mirage Control Loop

Example deployment used in this flow:

- Mirage remains orchestration-only and does not host seeds directly.
- Mirage may pair with one co-located local Ghost that shares network identity and locality context.
- Any Ghost can host any seed type; locality is a scheduling preference, not a placement restriction.
- The local Ghost can host critical/locality-sensitive seeds (for example `raft-node`) and local services (for example `mongodb`).
- External Ghosts can simultaneously host the same seed types with their own network URIs.
- In a distributed `raft-node` deployment, multiple Ghosts may each expose their own `raft-node` entrypoint.
- In a distributed `kdht-node` deployment, multiple Ghosts may expose `kdht-node` entrypoints for routing, key lookup, and replication.
- Mirage may schedule storage movement across the network between local and remote database seed URIs.
- Locality-aware control-loop diagram: [`models/control_loop_locality.mmd`](models/control_loop_locality.mmd)
