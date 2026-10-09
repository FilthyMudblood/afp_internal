# AgentEvalSci Fall 2026 — submission packet

Venue: [Agent Evaluation Science Fall 2026](https://evalscience.org/2026fall-nyc/submissions/), New York City, 20 November 2026.

OpenReview: [evalscience.org/AgentEvalSci/2026/Symposium](https://openreview.net/group?id=evalscience.org/AgentEvalSci/2026/Symposium)

This is a **short paper**, not an extended abstract. The paper evaluates host-level runtime containment on a planner–tool harness. The incremental claim is leftover telemetry, not a stronger first-time breaker.

## Checklist (CFP)

| Requirement | This packet |
|---|---|
| Track | Short paper (completed empirical analysis) |
| Length | 4–6 pages excluding references; appendices allowed and do not count |
| Template | Official AES LaTeX Template (`paper/`, XeLaTeX) |
| Author identity | **Not anonymous.** Muchen He, Independent Researcher |
| Archival | Presentation-only, non-archival |
| Prior work | Optional: cite the v1 Zenodo archive. Do not advertise the AIWILD rejection. |
| PDF | Single PDF via OpenReview |
| Presentation preference | Either (committee assigns oral or poster) |
| Attendance | Remote presentation is allowed for authors outside the US |

## Dates

- Abstract registration: **20 October 2026**
- Full PDF: **25 October 2026**
- Notification: 5 November 2026
- Symposium: 20 November 2026

## OpenReview fields

**Title.** AFP-Enterprise: Evaluating Host-Level Runtime Containment for Planner–Tool Execution

**Authors.** Muchen He (Independent Researcher). Add a contact email on the OpenReview record.

**Keywords.** Planner–tool runtime; Admission control; Pre-flight check; Retained telemetry; Stale-state retention; Circuit breaker; Multi-agent systems

**Abstract** (paste into OpenReview):

AFP-Enterprise is a host runtime kernel beside a local planner, reached by a synchronous pre-flight probe. Before irreversible tool I/O, the planner reports recursion depth and context, then blocks on allow, delay, or isolate. The kernel does not parse prompts or tool schemas. It checks, in order, an emergency stop, a depth cap, a four-axis pressure maximum, and a specified isolate–recover state machine. On a LangGraph planner with real lookup and HTTP GET tools, five bounded jobs complete under no gate, a per-step wipe, and the current sidecar (false isolate = 0). A scripted loop and a 48-GET burst complete without a gate and are contained by both gated variants. Those traces fire the depth cap and the hard pressure isolate; they do not reach the delay band or the recover machine. The two gated variants match on those jobs. They differ only when a later probe reuses leftover host telemetry: AFP-Enterprise refuses; the wipe allows. A new job that reports depth 0 is admitted under both. Two planners on one sidecar can overwrite each other, so the kernel keeps last-reported host telemetry, not a lock on who the planner is.

**Track.** Short Paper

**Presentation preference.** Either

## How to compile the PDF

The machine that drafted this packet has no XeLaTeX. Compile on Overleaf:

1. Zip the `paper/` folder (keep `fonts/`, `assets/`, `aes.sty`, `aes-author-year.bst`).
2. Upload the zip to Overleaf.
3. Set **Compiler = XeLaTeX**. pdfLaTeX will fail.
4. Set `main.tex` as the main document.
5. Recompile from scratch.
6. Check that the body is 4–6 pages before References. Appendices may continue after that.
7. If the body is under 4 pages, do not pad. If it exceeds 6, cut Related evaluations or move Table 1 notes into the appendix. Do not cut the three-way table or the construct non-claims.

Local compile, if TeX Live / MacTeX is installed:

```sh
cd artifacts/agentevalsci-2026/paper
latexmk -xelatex main.tex
```

Do **not** submit `main-review.tex`. The symposium is not anonymous.

## What changed relative to the NeurIPS AIWILD short paper

- Official AES template, named author.
- Lead with the company planner–tool scene and the name AFP-Enterprise. Same traces and the same honest claim boundary: increment = stale-state retention, not persistence.
- Mesh 100% stays in the appendix and is labeled an invariant.
- Do not mention the AIWILD rejection in the PDF. A rejected non-archival workshop paper is not a prior publication.

Do not edit the NeurIPS 4-page source for this venue.

## What you still do by hand

1. Add your contact email on OpenReview (and optionally `\aesemails` in `main.tex` if you want it on the PDF).
2. Register the abstract by 20 October, upload the PDF by 25 October.
3. After acceptance, register and present (remote is allowed from outside the US).
