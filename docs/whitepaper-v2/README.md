# Aegis Fabric Protocol Whitepaper v2.0 — Protocol Edition

企业 Path A 的实验与证据边界（不含 Exchange）见 **[AFP-Core 记录白皮书](../whitepaper-core/AFP-Core-记录白皮书.md)**。下面这份 v2 仍混有开放网格章节，不作 Core 存档。

## Read the whitepaper

**[whitepaper-v2-protocol-edition.md](./whitepaper-v2-protocol-edition.md)** — GitHub Protocol Edition (Draft v0.3)

Narrative-first English edition: full names in prose, mechanisms explained in-line, reader glossary with “short for” expansions.

Implementer diagrams, protobuf schemas, and code map: **[`ARCHITECTURE.md`](../../ARCHITECTURE.md)** at repository root (supplementary—not a substitute for the whitepaper body).

**v1** (empirical baseline, archived): [Zenodo 20674352](https://zenodo.org/records/20674352)

---

## Document split

| Track | Audience |
|-------|----------|
| **Protocol** (this whitepaper + root ARCHITECTURE) | Researchers, protocol engineers, GitHub readers |
| **Enterprise** (README, deploy/kubernetes) | Platform, security, compliance |

**Writing rules for the GitHub whitepaper:**

- Prefer **full names** in body prose (e.g. Single Execution Authority, Consequence Persistence Layer, Coordination Viability Probability).
- Glossary states what each short form is **short for**; do not force readers to memorize acronyms.
- Do not replace mechanism explanation with “see ARCHITECTURE”—put the control law in the whitepaper; ARCHITECTURE holds diagrams/schemas/code map.
- No Kubernetes nouns in whitepaper body; use “Policy Surface” abstraction.

---

## Contents (single document)

1. The Optimization Crisis
2. Consequence Persistence Layer
3. Pre-Intent Enforcement
4. Open-Network Topology
5. Governance Header & Wire Semantics
6. Empirical Baseline

---

## Archive

| File | Note |
|------|------|
| `archive/chapter-*.md` | Superseded split drafts — retained for history only; do not edit |
| `ARCHITECTURE.md` (this folder) | Deprecated pointer — use root [`ARCHITECTURE.md`](../../ARCHITECTURE.md) |
