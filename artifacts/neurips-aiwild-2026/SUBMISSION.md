# OpenReview submission checklist

Workshop: [Agents in the Wild @ NeurIPS 2026](https://agentwild-workshop.github.io/neurips2026/)

- Track: **Short Papers** (4 pages)
- Deadline: **29 August 2026, AoE**
- Notification: 29 September 2026, AoE
- Camera-ready: 4 December 2026, AoE
- Workshop: 11 or 12 December 2026, Sydney
- Portal: workshop OpenReview (create/activate profile by **15 August 2026** if you do not have one)
- Contact: agents-in-the-wild-neurips2026@googlegroups.com

## Before upload

1. Compile `paper/paper.tex` with `\usepackage[dblblindworkshop]{neurips_2026}`.
2. Confirm the body (title through Discussion) is **≤ 4 pages**.
3. Grep the PDF for identifying strings: `Aegis`, `AFP`, `FilthyMudblood`, `zenodo`, `Muchen`, `He,`.
4. Do **not** attach a GitHub URL or a named code dump. If you must share code, strip names and upload an anonymous zip with no author metadata.
5. No NeurIPS paper checklist (workshop says it is not required).

## OpenReview fields (draft)

**Title.** Persistent Runtime Brakes for Multi-Agent Coordination

**Abstract.** (paste from `paper.tex`, one paragraph)

Agent frameworks can plan and protocols can move messages, but neither answers a prior question at the execution boundary: may this coordination step run, and does a refusal still bind after the request ends? We argue that many multi-agent failures are physical before they are semantic---stacked delegation can exhaust a node while every hop remains well-formed. This short paper specifies a consequence persistence layer (CPL): an out-of-band gate that adjudicates before irreversible intent, stores allow / delay / isolate on identity, and can wrap an existing planner runtime. On a LangGraph planner that issues real tool I/O (local lookup and HTTP GET) through a sidecar pre-flight path, a one-tool job and an eight-tool job both complete with or without the gate (false isolate = 0); unbounded tick stacking reaches a 40-step cap without the gate and is isolated at depth 11 with it; a 48-task burst is refused at entropy 0.96. The reported table uses a deterministic tool-calling policy so the trial cannot flake; the same graph accepts a live OpenAI-compatible chat model. A synthetic mesh in the appendix is a sanity check, not the main result: there, 100% survival is an invariant of refusing increments past 0.95 when death is load ≥ 1. CPL is a translation of admission control and circuit breaking onto agent trajectories, not a replacement for prompt-injection defense.

**Keywords.** multi-agent safety; runtime control; protocol security; coordination reliability; agent infrastructure

**TL;DR.** Multi-agent stacks need a persistent pre-intent brake, not only semantic protocols. Path A on a LangGraph with real tools stops a recursion runaway and a burst the same graph otherwise completes; short and eight-tool jobs still finish. Mesh 100% is a breaker invariant, not field evidence.

## Dual-submission / archival

NeurIPS AIWILD was **non-archival**. AAMAS 2027 Blue Sky accepted papers **appear in the conference proceedings** (archival). Do not submit the same PDF as a full paper elsewhere under a dual-submission policy that forbids archival overlap. The Zenodo whitepaper is a technical report, not a major-venue publication; keep the anonymous PDF free of identifying links.

## After the AIWILD reject (next venue)

NeurIPS 2026 AIWILD short papers: **Reject** (reviews 5 / 9 / 6). Do not resubmit this draft there.

**Primary: AAMAS 2027 Blue Sky Ideas** (Hanoi, 3–7 May 2027). Four pages in AAMAS format, references extra. Abstract 5 Nov 2026 AoE; paper 12 Nov 2026 AoE. Author OpenReview registration 20 Oct 2026. CFP: https://warwick.ac.uk/fac/sci/dcs/aamas2027/calls/call-for-blue-sky-ideas/

Fit: vision track, finished results not required, same 4-page cap as this draft. Retune the lead toward the missing-layer diagnosis; keep the LangGraph table as one grounded example, not as a main-track experiment. Restyle to the AAMAS ACM template. Accepted papers are **archival** (conference proceedings).

Mandatory extra fields with the PDF: (1) why Blue Sky rather than main track; (2) author track record, visible only to chairs. Independent-researcher record is a real risk on (2).

**Backup: AAAI-27 workshops** (Montréal, 22–23 Feb 2027). Organizer submission deadline 20 Nov 2026; pick a matching CFP after the 2 Oct 2026 workshop CFP list is posted.

**Later: ICLR 2027 workshops** (suggested contribution deadline 1 Feb 2027). Use this if the next six weeks should go to mixed LLM / MCP traces instead of a November deadline.

Do not add rumor/gossip eval or unfreeze architecture for this retune.

### Blue Sky form drafts (fill on OpenReview)

**Rationale for Blue Sky** (visible to reviewers). Suggested starting text:

This is a layer diagnosis, not a completed empirical study. Agent frameworks and session protocols populate planning and transport; they do not bind a pre-intent, identity-persistent refuse at the execution boundary. The LangGraph traces show that Path A can wrap an existing planner; they are not a claim of in-the-wild safety. The missing object is a community agenda item (persistent runtime law for multi-agent coordination), which is why the submission belongs on Blue Sky rather than the main track.

**Author track record** (chairs only). Independent researcher; protocol specification and a public technical report. Do not invent affiliations. Keep this short and factual.

## After acceptance

See `AUTHOR_NOTES.md`: switch to `\usepackage[dblblindworkshop, final]{neurips_2026}`, restore author block, and optionally restore the AFP name.
