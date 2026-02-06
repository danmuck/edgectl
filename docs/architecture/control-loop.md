# Mirage Control Loop

Example deployment used in this flow:
- Mirage control plane runs locally with a local `raft-node` seed.
- One ghost server exposes `mongodb` and `raft-node` seeds.

```mermaid
flowchart LR
    U["User"]
    I["Intent"]
    R["Reconciliation"]
    C["Command/Event Loop"]
    S["State Update"]

    subgraph G["Mirage (Local)"]
        GR["raft-node seed"]
    end

    subgraph SEED["Ghost Server"]
        SM["mongodb seed"]
        SR["raft-node seed"]
    end

    U --> I
    I --> R
    R --> C
    C --> S
    S --> R

    C -->|"Command: start/stop/configure"| SM
    C -->|"Command: join/replicate"| SR
    C -->|"Command: join/replicate"| GR

    SM -->|"Event: status/health/metrics"| C
    SR -->|"Event: term/leader/commit"| C
    GR -->|"Event: term/leader/commit"| C
```
