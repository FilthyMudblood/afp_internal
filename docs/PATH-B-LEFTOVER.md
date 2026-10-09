# Path B leftover in this sidecar

This repo is **AFP-Path-A** (enterprise). The sidecar still **compiles** a few Path B surfaces so PreFlight and ingress share one SEA. That is leftover wiring, not AFP-Exchange.

Do not delete these packages without a dedicated extract. Copies for Exchange writing live in [`../../AFP-Path-B/reference-code/`](../../AFP-Path-B/reference-code/).

| Path | Path B surface |
|------|----------------|
| `internal/topology/` | Neighbor store, gossip, DID |
| `api/afp/v1/governance.proto` | GovernanceHeader |
| `internal/dataplane/ingress.go` | Header-before-payload admit |
| `internal/dataplane/egress.go` | Outbound header attach |
| `cmd/modetester/` | Open-exchange probe |
| `cmd/node_z/` | Peer node probe |
| `cmd/simulator/` | 500-node Monte Carlo (not a Path A claim) |

Evaluated Path A work is PreFlight + Python SDK traces, not gossip or stranger tax.
