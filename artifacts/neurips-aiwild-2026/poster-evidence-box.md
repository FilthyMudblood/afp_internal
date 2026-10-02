# Poster box: what counts as evidence

Use this as a two-column panel. Title on the poster: **Speech is not evidence. The next commit is.**

## The line you say while pointing at the box

Most evals grade how the agent *talks* or whether it still *obeys*. Those are easy to fake. We grade the *next step* before it commits, and we keep a record you can point to.

## The box

| Superficial check (not enough) | Solid evidence (what we want) |
|---|---|
| The prompt looks nasty or jailbreak-like | A pre-flight record of the **pending** step: who, what hop, estimated fan-out, depth, pressure |
| The agent *sounds* polite and cooperative | Manner is ignored. Polite text can still precede a illegal-as-coordination step |
| The agent refused a bad instruction once (“obedient”) | Obedience is not the act. The question is whether **this next commit** may run |
| Nothing bad has finished yet, so it must be fine | Too late if you wait for the finished crime. Look at **what is about to run** |
| A human rater’s vibe: “this seems shady” | An objective line that was crossed or about to be crossed, written down |

**Illegal here means operational, not “the model had evil thoughts”:**

- **Policy-illegal:** this identity may not use this tool / corpus / boundary.
- **Physics-illegal:** this step would push depth, memory, or neighbor load past a published line.
- **Evidence:** the pre-intent trace (allow / delay / isolate) plus the numbers that produced it.

If you cannot show that trace, you do not have a benchmark. You have a conversation.

## Tiny caption under the box

A valid, polite MCP call can still be a step that must not run. We do not parse the prompt. We adjudicate the commit and keep the receipt.
