# NeurIPS 2026 Agents in the Wild — short-paper kit

Anonymous 4-page short paper. NeurIPS 2026 AIWILD: rejected. Next target and dates: [`SUBMISSION.md`](SUBMISSION.md) (AAMAS 2027 Blue Sky).

- Chinese notes: [`投稿说明.md`](投稿说明.md)
- OpenReview fields / venue pass: [`SUBMISSION.md`](SUBMISSION.md)
- Camera-ready name restore: [`AUTHOR_NOTES.md`](AUTHOR_NOTES.md)
- LaTeX: [`paper/paper.tex`](paper/paper.tex)

Reproduce appendix mesh tables:

```bash
go run ./scripts/sweep.go -out results
```

LangGraph Path A traces (from repo root; sidecar auto-spawned):

```bash
PYTHONPATH=sdk/python python sdk/python/examples/runtime_trace.py \
  --out artifacts/neurips-aiwild-2026/results
PYTHONPATH=sdk/python python sdk/python/examples/llm_tool_trace.py \
  --out artifacts/neurips-aiwild-2026/results
# optional live model (OpenAI-compatible):
# OPENAI_BASE_URL=http://127.0.0.1:11434/v1 OPENAI_MODEL=llama3.1 \
#   PYTHONPATH=sdk/python python sdk/python/examples/llm_tool_trace.py --backend live
```
