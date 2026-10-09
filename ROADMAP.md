# AFP Roadmap

> **Code freeze:** Architecture intent is complete through **PR-6c**. No further feature PRs until whitepaper v2.0 and production prioritization are settled. Items below are **Production Hardening (Phase 3+)** — not blockers for PoC, demo, or internal pilot.

## Shipped (Phase 1 + Phase 2)

| Area | Delivered |
|------|-----------|
| Data plane | Sidecar · UDS IPC · PreFlightCheck · EntropyMonitor · ACC/FSM |
| Application layer | Python SDK · LangGraph `@afp_governed_node` · annotate graceful degrade |
| Kubernetes | Sidecar co-deploy · `AFPClusterPolicy` CRD · Operator · ConfigMap hot-reload |
| Runtime control | Policy Controller · `StreamPolicyUpdates` · Operator bridge · Kill Switch CLI |
| Security | SA TokenReview · dev mTLS · revision replay · status writeback · delete propagation |
| Distribution | GHCR images · `make kind-quickstart` · demo-agent |

---

## Phase 3 — Production Hardening

### P0 — Security & transport

| Item | Why | Notes |
|------|-----|-------|
| **cert-manager integration** | Rotate policy-stream TLS without manual `generate_policy_tls.sh` | Replace dev CA with cluster CA / Let's Encrypt internal |
| **External policyctl auth** | port-forward callers lack SA tokens today | OIDC, kubeconfig impersonation, or break-glass RBAC |
| **Production crypto verification** | Governance headers use simplified validation | Full CVP / signature chain from v1 whitepaper |

### P1 — Data plane enforcement

| Item | Why | Notes |
|------|-----|-------|
| **iptables / eBPF socket hijack** | Egress currently trust-based on localhost | `NET_ADMIN` hook already reserved in manifests |
| **Real cgroup / memory sampling** | Entropy uses mock/simplified providers | Wire `/sys/fs/cgroup` for OOM-adjacent pressure |
| **Ingress → agent forwarding** | Sidecar accepts TCP but does not proxy to agent runtime | Complete `io.Copy` path in `cmd/dataplane/sidecar` |

### P2 — Control plane & ops

| Item | Why | Notes |
|------|-----|-------|
| **Helm chart (production)** | Skeleton only today | Templatize demo + generic agent profiles |
| **Multi-cluster policy federation** | Single control plane per cluster | Aggregate `AFPClusterPolicy` + cross-mesh stream fan-out |
| **Operator metrics & alerts** | Status subresource exists; no Prometheus exporter | Reconcile latency, stream publish failures |
| **CRD finalizers** | Delete propagation works; optional cleanup of ConfigMaps | Explicit orphan ConfigMap GC policy |

### P3 — Mesh & research

| Item | Why | Notes |
|------|-----|-------|
| **P2P gossip transport** | Inbound ed25519 verify + unsigned drop **done**; UDP/TCP fan-out TODO | Full topological quarantine at scale |
| **Open-exchange stranger tax** | Mode exists; hardening for untrusted peers | Collateral / probation FSM production paths |
| **Formal verification** | Monte Carlo empirical proof exists | TLA+ / model-check ACC kernel bounds |

---

## Phase 4 — Ecosystem

| Item | Description |
|------|-------------|
| **CrewAI / AutoGen adapters** | Same SDK pattern as LangGraph |
| **Argent Signaling Protocol (ASP) interop doc** | Position AFP relative to ASP and other application-layer protocols |
| **SLO templates** | `afp_preflight_actions_total` → Grafana dashboards + alert rules |
| **Whitepaper v2.0 Zenodo DOI** | Mixed Protocol Edition archived: [`../AFP-Archived/mixed-whitepaper-v2/whitepaper-v2-protocol-edition.md`](../AFP-Archived/mixed-whitepaper-v2/whitepaper-v2-protocol-edition.md) · v1: [Zenodo](https://zenodo.org/records/20674352) |

---

## Explicit non-goals (for now)

- Rewriting LangGraph / CrewAI internals
- Replacing Kubernetes with a custom scheduler
- On-chain economics (CVP collateral remains architectural, not deployed)

---

## How to propose work

1. Open a GitHub Issue tagged `phase-3` with the roadmap row reference.
2. Link to whitepaper v2.0 section if the item changes the theoretical model.
3. PoC and demo paths must remain working on `main` without Phase 3 deps.
