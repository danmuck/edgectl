# Mirage Control Loop

Example deployment used in this flow:
- Mirage remains orchestration-only and does not host seeds directly.
- Mirage may pair with one co-located local Ghost that shares network identity and locality context.
- The local Ghost can host critical/locality-sensitive seeds (for example `raft-node`).
- External Ghosts host additional service seeds (for example `mongodb`).
- Locality-aware control-loop diagram: [`models/control_loop_locality.mmd`](models/control_loop_locality.mmd)
