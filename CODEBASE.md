# AFP Codebase Map

> Quick navigation for a frozen-but-readable repository (PR-6c+).
>
> **Protocol specification:** see [ARCHITECTURE.md](ARCHITECTURE.md) — stack model, CPL/SEA/CVP, dual-path enforcement.

## Top-level layout

```text
aegis-fabric/
├── api/afp/v1/              # Protobuf contracts (buf generate → pkg/protocol)
├── cmd/                       # Binaries — see cmd/README.md
│   ├── dataplane/             # Sidecar + IPC tools
│   ├── controlplane/          # Operator + policy stream
│   └── demo/                  # Monte Carlo & verification harnesses
├── internal/                  # Private Go packages (below)
├── pkg/protocol/v1/           # Generated protobuf Go code
├── sdk/python/                # Python SDK + LangGraph adapter
├── deploy/                    # Runtime manifests & observability stacks
│   ├── kubernetes/            # Production K8s (use this)
│   ├── demo/                  # docker-compose demo assets
│   └── monitor/               # Prometheus / Grafana local stack
├── scripts/                   # kind-quickstart, TLS, verification
├── Dockerfile                 # Sidecar image (dataplane + controlplane CLIs)
├── Dockerfile.operator        # Operator-only image
├── Dockerfile.demo-agent      # LangGraph demo agent
├── ROADMAP.md                 # Phase 3+ hardening (no new features on main)
├── ARCHITECTURE.md            # AFP protocol specification (L0–L5, CPL, SEA, CVP)
└── docs/whitepaper-core/      # Path A record only; mixed v2 is in ../AFP-Archived
```

---

## internal/ — by layer

| Package | Layer | Responsibility |
|---------|-------|----------------|
| `internal/dataplane` | L2 | Ingress, egress, PreFlight ACC, SEA authority |
| `internal/ipc` | L2 | UDS gRPC server for Python SDK |
| `internal/core` | L2 | EntropyMonitor, cgroup/memory sampling |
| `internal/control` | L2 | ACC math kernel, FSM (not K8s — naming legacy) |
| `internal/config` | L2/L3 | Env config, RuntimePolicy, fsnotify, stream overlay |
| `internal/policyplane` | L3 | gRPC stream hub, mTLS, TokenReview auth |
| `internal/controller` | L3 | K8s operator: CRD → ConfigMap + publish + status |
| `internal/topology` | L4 | Neighbor store, gossip, DID resolution |
| `internal/telemetry` | Ops | Prometheus metrics |

**Mental model:** `control` = math · `controller` = Kubernetes · `policyplane` = gRPC stream · `dataplane` = packets + preflight

---

## deploy/ — what to apply

| Path | Use when |
|------|----------|
| `deploy/kubernetes/` | **Production / kind quickstart** — CRD, operator, demo pod |
| `deploy/demo/` | Local docker-compose L7 demo |
| `deploy/monitor/` | Prometheus + Grafana for `make demo-report` |
| `deploy/k8s/` | Legacy single-file pod sample (prefer `kubernetes/`) |

---

## Common commands (after reorg)

```bash
# Production binaries
go run ./cmd/dataplane/sidecar
go run ./cmd/controlplane/operator
go run ./cmd/controlplane/policy-controller
go run ./cmd/controlplane/policyctl --kill-switch

# Quick verification
go run ./cmd/dataplane/preflightclient --recursion-depth 12
go run ./cmd/demo/simulator

# Full K8s demo
make kind-quickstart
```

---

## Docker images → binaries

| Image | Entrypoints |
|-------|-------------|
| `aegis-fabric-sidecar` | `sidecar`, `policy-controller`, `preflightclient`, `policyctl`, … |
| `aegis-fabric-operator` | `operator` |
| `afp-demo-agent` | LangGraph Python demo |

---

## What not to touch (frozen)

See [ROADMAP.md](ROADMAP.md). New feature work belongs in Phase 3 issues, not drive-by refactors.
