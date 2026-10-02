# Runtime traces (CPL on vs off)

Paired LangGraph Path A. Sidecar auto-spawned.

Depth-only (no tools):

```bash
PYTHONPATH=sdk/python python sdk/python/examples/runtime_trace.py \
  --out artifacts/neurips-aiwild-2026/results
```

Planner + real tools (lookup, HTTP GET). Default backend is a deterministic tool-calling policy (CI / paper table). S3/S4 are graph-driven so they do not depend on a model looping.

```bash
PYTHONPATH=sdk/python python sdk/python/examples/llm_tool_trace.py \
  --out artifacts/neurips-aiwild-2026/results
```

Live OpenAI-compatible model for S1/S2 (Ollama or a hosted key):

```bash
OPENAI_BASE_URL=http://127.0.0.1:11434/v1 \
OPENAI_MODEL=llama3.1 \
PYTHONPATH=sdk/python python sdk/python/examples/llm_tool_trace.py \
  --backend live --out artifacts/neurips-aiwild-2026/results
```

Writes `runtime_trace.csv` / `llm_tool_trace.csv` and matching JSON.
