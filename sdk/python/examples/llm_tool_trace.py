"""Paired LangGraph traces: real tools, scripted or live LLM, CPL on vs off.

Pre-flight sits before tool I/O (and before the next planner hop).
S3/S4 are graph-driven so runaway/burst do not depend on the model looping.

Run (from repo root, sidecar auto-spawned per job):
  PYTHONPATH=sdk/python python sdk/python/examples/llm_tool_trace.py \\
    --out artifacts/neurips-aiwild-2026/results
  PYTHONPATH=sdk/python python sdk/python/examples/llm_tool_trace.py --backend live
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import socket
import sys
import time
import urllib.error
import urllib.request
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread
from typing import Any, Iterator

from langgraph.errors import GraphRecursionError
from langgraph.graph import END, StateGraph

from afp_sdk import AFPSidecarClient
from afp_sdk.exceptions import AFPQuotaExceededError
from afp_sdk.langgraph import afp_governed_node
from afp_sdk.testing import REPO_ROOT, public_reason, start_sidecar, stop_sidecar

LOOKUP: dict[str, str] = {
    "capital:france": "Paris",
    "capital:japan": "Tokyo",
    "capital:kenya": "Nairobi",
    "capital:peru": "Lima",
}

STORM_N = 48  # burst / MaxToolConcurrency(50) => entropy >= 0.95
GRAPH_CAP_RUNAWAY = 40
DEPTH_CAP = 10

TOOL_SCHEMAS = [
    {
        "type": "function",
        "function": {
            "name": "lookup",
            "description": "Read a local fact by key.",
            "parameters": {
                "type": "object",
                "properties": {"key": {"type": "string"}},
                "required": ["key"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "fetch",
            "description": "HTTP GET a local path.",
            "parameters": {
                "type": "object",
                "properties": {"path": {"type": "string"}},
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "expand",
            "description": "Schedule n follow-up fetches (burst).",
            "parameters": {
                "type": "object",
                "properties": {"n": {"type": "integer"}},
                "required": ["n"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "tick",
            "description": "Continue stacking one more hop.",
            "parameters": {"type": "object", "properties": {}},
        },
    },
]


@dataclass
class JobSpec:
    name: str
    prompt: str
    script: list[dict[str, Any]] = field(default_factory=list)
    expected: dict[str, str] = field(default_factory=dict)
    graph_stop_depth: int = 16
    recursion_limit: int = 40
    live_ok: bool = False


JOBS: dict[str, JobSpec] = {
    "s1_short": JobSpec(
        name="s1_short",
        prompt="What is the capital of France? Use lookup with key capital:france, then stop.",
        script=[{"name": "lookup", "args": {"key": "capital:france"}}],
        expected={"capital:france": "Paris"},
        graph_stop_depth=8,
        recursion_limit=20,
        live_ok=True,
    ),
    "s2_long": JobSpec(
        name="s2_long",
        prompt=(
            "Collect eight facts then stop. "
            "lookup keys capital:france, capital:japan, capital:kenya, capital:peru. "
            "fetch paths /item/1, /item/2, /item/3, /item/4."
        ),
        script=[
            {"name": "lookup", "args": {"key": "capital:france"}},
            {"name": "fetch", "args": {"path": "/item/1"}},
            {"name": "lookup", "args": {"key": "capital:japan"}},
            {"name": "fetch", "args": {"path": "/item/2"}},
            {"name": "lookup", "args": {"key": "capital:kenya"}},
            {"name": "fetch", "args": {"path": "/item/3"}},
            {"name": "lookup", "args": {"key": "capital:peru"}},
            {"name": "fetch", "args": {"path": "/item/4"}},
        ],
        expected={
            "capital:france": "Paris",
            "capital:japan": "Tokyo",
            "capital:kenya": "Nairobi",
            "capital:peru": "Lima",
            "/item/1": "payload-1",
            "/item/2": "payload-2",
            "/item/3": "payload-3",
            "/item/4": "payload-4",
        },
        graph_stop_depth=16,
        recursion_limit=30,
        live_ok=True,
    ),
    "s2_lookups": JobSpec(
        name="s2_lookups",
        prompt="Lookup capitals of France, Japan, Kenya, and Peru, then stop.",
        script=[
            {"name": "lookup", "args": {"key": "capital:france"}},
            {"name": "lookup", "args": {"key": "capital:japan"}},
            {"name": "lookup", "args": {"key": "capital:kenya"}},
            {"name": "lookup", "args": {"key": "capital:peru"}},
        ],
        expected={
            "capital:france": "Paris",
            "capital:japan": "Tokyo",
            "capital:kenya": "Nairobi",
            "capital:peru": "Lima",
        },
        graph_stop_depth=12,
        recursion_limit=24,
    ),
    "s2_fetches": JobSpec(
        name="s2_fetches",
        prompt="Fetch /item/1 through /item/6, then stop.",
        script=[{"name": "fetch", "args": {"path": f"/item/{i}"}} for i in range(1, 7)],
        expected={f"/item/{i}": f"payload-{i}" for i in range(1, 7)},
        graph_stop_depth=16,
        recursion_limit=30,
    ),
    "s2_repeat": JobSpec(
        name="s2_repeat",
        prompt="Lookup capital:france eight times, then stop.",
        script=[{"name": "lookup", "args": {"key": "capital:france"}}] * 8,
        expected={"capital:france": "Paris"},
        graph_stop_depth=20,
        recursion_limit=30,
    ),
    "s3_runaway": JobSpec(
        name="s3_runaway",
        prompt="Keep calling tick until stopped.",
        graph_stop_depth=GRAPH_CAP_RUNAWAY,
        recursion_limit=120,
    ),
    "s4_storm": JobSpec(
        name="s4_storm",
        prompt=f"Call expand with n={STORM_N}, then fetch every queued path.",
        graph_stop_depth=STORM_N + 16,
        recursion_limit=220,
    ),
}


@dataclass
class ToolTraceResult:
    job: str
    mode: str
    backend: str
    seed: int
    task_completed: bool
    false_isolated: bool
    planner_steps: int
    tool_calls: int
    preflight_probes: int
    allow: int
    delay: int
    isolate_count: int
    isolated: bool
    block_reason: str
    final_depth: int
    wall_ms: float
    prompt_tokens: int


class _FetchHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path.startswith("/storm/"):
            body = f"storm-{self.path.rsplit('/', 1)[-1]}".encode()
        elif self.path.startswith("/item/"):
            body = f"payload-{self.path.rsplit('/', 1)[-1]}".encode()
        else:
            body = b"missing"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A003
        return


@contextmanager
def fetch_server() -> Iterator[str]:
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _FetchHandler)
    thread = Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    host, port = httpd.server_address[:2]
    try:
        yield f"http://{host}:{port}"
    finally:
        httpd.shutdown()
        thread.join(timeout=2)


def _next_scripted_action(job: JobSpec, state: dict[str, Any]) -> dict[str, Any] | None:
    if job.name == "s3_runaway":
        return {"name": "tick", "args": {}}
    if job.name == "s4_storm":
        queue = list(state.get("fetch_queue") or [])
        if not state.get("expanded"):
            return {"name": "expand", "args": {"n": STORM_N}}
        if queue:
            return {"name": "fetch", "args": {"path": queue[0]}}
        return None
    script = job.script
    idx = int(state.get("script_i", 0))
    if idx >= len(script):
        return None
    return dict(script[idx])


def _openai_chat(
    messages: list[dict[str, Any]],
) -> tuple[dict[str, Any], int]:
    base = os.getenv("OPENAI_BASE_URL", "http://127.0.0.1:11434/v1").rstrip("/")
    model = os.getenv("OPENAI_MODEL", "qwen2.5:0.5b")
    key = os.getenv("OPENAI_API_KEY", "ollama")
    payload = {
        "model": model,
        "temperature": 0,
        "max_tokens": 128,
        "messages": messages,
        "tools": TOOL_SCHEMAS,
    }
    request = urllib.request.Request(
        f"{base}/chat/completions",
        data=json.dumps(payload).encode(),
        headers={
            "Authorization": f"Bearer {key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=180) as resp:
            data = json.loads(resp.read().decode())
    except (TimeoutError, socket.timeout) as exc:
        raise RuntimeError(f"live LLM timed out at {base}: {exc}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"live LLM unreachable at {base}: {exc}") from exc
    usage = data.get("usage") or {}
    tokens = int(usage.get("prompt_tokens") or 0)
    choices = data.get("choices") or []
    if not choices:
        raise RuntimeError("live LLM returned no choices")
    return choices[0].get("message") or {}, tokens


def _live_action(
    job: JobSpec, state: dict[str, Any]
) -> tuple[dict[str, Any] | None, int]:
    chat = list(state.get("chat") or [])
    if not chat:
        chat = [
            {
                "role": "system",
                "content": (
                    "You are a tool-using planner. Call tools to collect facts. "
                    "Do not invent facts. Stop when the user request is satisfied."
                ),
            },
            {"role": "user", "content": job.prompt},
        ]
    message, tokens = _openai_chat(chat)
    chat.append(message)
    state["chat"] = chat
    tool_calls = message.get("tool_calls") or []
    if not tool_calls:
        return None, tokens
    call = tool_calls[0]
    fn = call.get("function") or {}
    name = str(fn.get("name") or "")
    raw_args = fn.get("arguments") or "{}"
    args = json.loads(raw_args) if isinstance(raw_args, str) else dict(raw_args)
    return {"name": name, "args": args}, tokens


def _execute_tool(
    name: str, args: dict[str, Any], state: dict[str, Any], base_url: str
) -> dict[str, Any]:
    facts = dict(state.get("facts") or {})
    messages = list(state.get("messages") or [])
    updates: dict[str, Any] = {
        "tool_calls": int(state.get("tool_calls", 0)) + 1,
        "pending_tool": None,
    }
    if name == "lookup":
        key = str(args.get("key") or "")
        value = LOOKUP.get(key, "unknown")
        facts[key] = value
        messages.append(f"lookup {key}={value}")
        updates["script_i"] = int(state.get("script_i", 0)) + 1
    elif name == "fetch":
        path = str(args.get("path") or "")
        url = base_url.rstrip("/") + path
        with urllib.request.urlopen(url, timeout=2) as resp:
            body = resp.read().decode()
        facts[path] = body
        messages.append(f"fetch {path}={body}")
        queue = list(state.get("fetch_queue") or [])
        if queue and queue[0] == path:
            updates["fetch_queue"] = queue[1:]
        updates["script_i"] = int(state.get("script_i", 0)) + 1
    elif name == "tick":
        updates["ticks"] = int(state.get("ticks", 0)) + 1
        messages.append(f"tick-{updates['ticks']}")
    elif name == "expand":
        n = int(args.get("n") or STORM_N)
        updates["expanded"] = True
        updates["fetch_queue"] = [f"/storm/{i}" for i in range(n)]
        messages.append(f"expand n={n}")
    else:
        messages.append(f"unknown-tool {name}")
    updates["facts"] = facts
    updates["messages"] = messages
    updates["context_memory_bytes"] = max(64, sum(len(m) for m in messages) * 64)
    if state.get("chat"):
        chat = list(state["chat"])
        chat.append({"role": "tool", "content": messages[-1] if messages else ""})
        updates["chat"] = chat
    return updates


def _estimated_tasks(action: dict[str, Any] | None) -> int:
    if not action:
        return 1
    if action.get("name") == "expand":
        return int((action.get("args") or {}).get("n") or STORM_N)
    return 1


def _record_preflight(stats: dict[str, int], fn: Any) -> Any:
    stats["preflight_probes"] += 1
    slept = {"s": 0.0}
    orig_sleep = time.sleep

    def spy(seconds: float) -> None:
        slept["s"] += float(seconds)
        orig_sleep(seconds)

    time.sleep = spy  # type: ignore[assignment]
    try:
        result = fn()
    except AFPQuotaExceededError:
        stats["isolate_count"] += 1
        raise
    finally:
        time.sleep = orig_sleep
    if slept["s"] > 0:
        stats["delay"] += 1
    else:
        stats["allow"] += 1
    return result


def build_app(
    *,
    job: JobSpec,
    governed: bool,
    backend: str,
    stats: dict[str, int],
    token_box: dict[str, int],
    base_url: str,
):
    def planner_body(state: dict[str, Any]) -> dict[str, Any]:
        depth = int(state.get("recursion_depth", 0)) + 1
        tokens = 0
        if backend == "live" and job.live_ok:
            try:
                action, tokens = _live_action(job, state)
            except RuntimeError as exc:
                return {
                    **state,
                    "recursion_depth": depth,
                    "planner_steps": int(state.get("planner_steps", 0)) + 1,
                    "pending_tool": None,
                    "done": True,
                    "afp_block_reason": str(exc),
                    "context_memory_bytes": max(
                        64, sum(len(str(m)) for m in (state.get("messages") or [])) * 64
                    ),
                }
        else:
            action = _next_scripted_action(job, state)
        token_box["prompt_tokens"] += tokens
        done = action is None
        if job.name == "s3_runaway" and depth >= job.graph_stop_depth:
            done = True
            action = None
        return {
            **state,
            "recursion_depth": depth,
            "planner_steps": int(state.get("planner_steps", 0)) + 1,
            "pending_tool": action,
            "done": done,
            "context_memory_bytes": max(
                64, sum(len(str(m)) for m in (state.get("messages") or [])) * 64
            ),
        }

    if governed:
        gated = afp_governed_node(planner_body, on_quota_exceeded="raise")

        def planner(state: dict[str, Any]) -> dict[str, Any]:
            def run() -> dict[str, Any]:
                return gated(state)

            try:
                return _record_preflight(stats, run)
            except AFPQuotaExceededError as exc:
                return {
                    **state,
                    "afp_blocked": True,
                    "afp_block_reason": str(exc),
                    "pending_tool": None,
                    "done": True,
                }

    else:

        def planner(state: dict[str, Any]) -> dict[str, Any]:
            return planner_body(state)

    def tools(state: dict[str, Any]) -> dict[str, Any]:
        action = state.get("pending_tool") or {}
        name = str(action.get("name") or "")
        args = dict(action.get("args") or {})
        estimated = _estimated_tasks(action if action else None)

        def execute() -> dict[str, Any]:
            updates = _execute_tool(name, args, state, base_url)
            return {**state, **updates}

        if not governed:
            return execute()

        client = AFPSidecarClient()

        def probe_and_run() -> dict[str, Any]:
            client.report_state(
                current_recursion_depth=int(state.get("recursion_depth", 0)),
                context_memory_bytes=int(state.get("context_memory_bytes") or 0),
            )
            client.pre_flight_check(
                estimated_tasks=estimated,
                trace_id=str(state.get("trace_id") or ""),
            )
            return execute()

        try:
            return _record_preflight(stats, probe_and_run)
        except AFPQuotaExceededError as exc:
            return {
                **state,
                "afp_blocked": True,
                "afp_block_reason": str(exc),
                "pending_tool": None,
                "done": True,
            }

    def route(state: dict[str, Any]) -> str:
        if state.get("afp_blocked") or state.get("done"):
            return "stop"
        if int(state.get("recursion_depth", 0)) >= job.graph_stop_depth:
            return "stop"
        if state.get("pending_tool"):
            return "tools"
        return "planner"

    graph = StateGraph(dict)
    graph.add_node("planner", planner)
    graph.add_node("tools", tools)
    graph.set_entry_point("planner")
    graph.add_conditional_edges(
        "planner", route, {"planner": "planner", "tools": "tools", "stop": END}
    )
    graph.add_conditional_edges(
        "tools", route, {"planner": "planner", "tools": "tools", "stop": END}
    )
    return graph.compile()


def _task_completed(job: JobSpec, state: dict[str, Any], isolated: bool) -> bool:
    if isolated:
        return False
    if job.name == "s3_runaway":
        return int(state.get("recursion_depth", 0)) >= job.graph_stop_depth
    if job.name == "s4_storm":
        facts = state.get("facts") or {}
        return sum(1 for key in facts if str(key).startswith("/storm/")) >= STORM_N
    facts = state.get("facts") or {}
    return all(facts.get(key) == value for key, value in job.expected.items())


def run_once(
    *,
    job: JobSpec,
    governed: bool,
    backend: str,
    seed: int,
    base_url: str,
) -> ToolTraceResult:
    AFPSidecarClient.reset_singleton()
    stats = {
        "preflight_probes": 0,
        "allow": 0,
        "delay": 0,
        "isolate_count": 0,
    }
    token_box = {"prompt_tokens": 0}
    app = build_app(
        job=job,
        governed=governed,
        backend=backend,
        stats=stats,
        token_box=token_box,
        base_url=base_url,
    )
    state: dict[str, Any] = {
        "recursion_depth": 0,
        "messages": [job.prompt],
        "context_memory_bytes": 64,
        "planner_steps": 0,
        "tool_calls": 0,
        "script_i": 0,
        "facts": {},
        "trace_id": f"{job.name}-{'on' if governed else 'off'}-{seed}",
        "seed": seed,
    }
    started = time.perf_counter()
    try:
        final = app.invoke(state, config={"recursion_limit": job.recursion_limit})
    except AFPQuotaExceededError as exc:
        wall_ms = (time.perf_counter() - started) * 1000.0
        return ToolTraceResult(
            job=job.name,
            mode="on" if governed else "off",
            backend=backend,
            seed=seed,
            task_completed=False,
            false_isolated=job.live_ok,
            planner_steps=int(state.get("planner_steps", 0)),
            tool_calls=int(state.get("tool_calls", 0)),
            preflight_probes=stats["preflight_probes"],
            allow=stats["allow"],
            delay=stats["delay"],
            isolate_count=max(stats["isolate_count"], 1),
            isolated=True,
            block_reason=public_reason(str(exc)),
            final_depth=int(state.get("recursion_depth", 0)),
            wall_ms=round(wall_ms, 2),
            prompt_tokens=token_box["prompt_tokens"],
        )
    except GraphRecursionError as exc:
        wall_ms = (time.perf_counter() - started) * 1000.0
        return ToolTraceResult(
            job=job.name,
            mode="on" if governed else "off",
            backend=backend,
            seed=seed,
            task_completed=False,
            false_isolated=False,
            planner_steps=int(state.get("planner_steps", 0)),
            tool_calls=int(state.get("tool_calls", 0)),
            preflight_probes=stats["preflight_probes"],
            allow=stats["allow"],
            delay=stats["delay"],
            isolate_count=stats["isolate_count"],
            isolated=False,
            block_reason=public_reason(str(exc).splitlines()[0]),
            final_depth=int(state.get("recursion_depth", 0)),
            wall_ms=round(wall_ms, 2),
            prompt_tokens=token_box["prompt_tokens"],
        )
    wall_ms = (time.perf_counter() - started) * 1000.0
    blocked = bool(final.get("afp_blocked"))
    completed = _task_completed(job, final, blocked)
    would_complete = _task_completed(job, final, isolated=False)
    return ToolTraceResult(
        job=job.name,
        mode="on" if governed else "off",
        backend=backend,
        seed=seed,
        task_completed=completed,
        false_isolated=blocked and would_complete,
        planner_steps=int(final.get("planner_steps", 0)),
        tool_calls=int(final.get("tool_calls", 0)),
        preflight_probes=stats["preflight_probes"],
        allow=stats["allow"],
        delay=stats["delay"],
        isolate_count=stats["isolate_count"],
        isolated=blocked,
        block_reason=public_reason(str(final.get("afp_block_reason") or "")),
        final_depth=int(final.get("recursion_depth", 0)),
        wall_ms=round(wall_ms, 2),
        prompt_tokens=token_box["prompt_tokens"],
    )


def write_outputs(
    rows: list[ToolTraceResult], out_dir: Path, stem: str = "llm_tool_trace"
) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    csv_path = out_dir / f"{stem}.csv"
    json_path = out_dir / f"{stem}.json"
    fieldnames = list(asdict(rows[0]).keys())
    with csv_path.open("w", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(asdict(row))
    json_path.write_text(json.dumps([asdict(r) for r in rows], indent=2) + "\n")
    print(f"wrote {csv_path}")
    print(f"wrote {json_path}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="CPL on/off LLM+tool traces")
    parser.add_argument(
        "--out",
        type=Path,
        default=REPO_ROOT / "artifacts/neurips-aiwild-2026/results",
    )
    parser.add_argument("--backend", choices=("scripted", "live"), default="scripted")
    parser.add_argument(
        "--jobs",
        default="",
        help="Comma-separated job names (default: all for scripted, S1/S2 for live)",
    )
    parser.add_argument("--seeds", type=int, default=1)
    parser.add_argument("--socket", default="")
    return parser.parse_args()


def selected_jobs(backend: str, raw: str) -> list[JobSpec]:
    if raw.strip():
        names = [part.strip() for part in raw.split(",") if part.strip()]
    elif backend == "live":
        names = ["s1_short", "s2_long"]
    else:
        names = ["s1_short", "s2_long", "s3_runaway", "s4_storm"]
    missing = [name for name in names if name not in JOBS]
    if missing:
        raise SystemExit(f"unknown jobs: {missing}")
    return [JOBS[name] for name in names]


def main() -> int:
    args = parse_args()
    jobs = selected_jobs(args.backend, args.jobs)
    rows: list[ToolTraceResult] = []
    with fetch_server() as base_url:
        for job in jobs:
            for seed in range(args.seeds):
                for governed in (False, True):
                    proc = None
                    socket_path = args.socket or (
                        f"/tmp/afp-llm-tool-{os.getpid()}-{job.name}-"
                        f"{'on' if governed else 'off'}-{seed}.sock"
                    )
                    try:
                        if not args.socket:
                            proc = start_sidecar(socket_path)
                        os.environ["AFP_IPC_SOCKET"] = socket_path
                        os.environ["AFP_SDK_FAIL_MODE"] = "closed"
                        row = run_once(
                            job=job,
                            governed=governed,
                            backend=args.backend if job.live_ok else "scripted",
                            seed=seed,
                            base_url=base_url,
                        )
                        rows.append(row)
                        print(
                            f"{row.job:10} mode={row.mode:3} backend={row.backend:8} "
                            f"completed={row.task_completed} tools={row.tool_calls:2} "
                            f"probes={row.preflight_probes:2} isolated={row.isolated} "
                            f"reason={row.block_reason[:60]}"
                        )
                    finally:
                        AFPSidecarClient.reset_singleton()
                        if not args.socket:
                            stop_sidecar(proc, socket_path)
    stem = "llm_tool_trace_live" if args.backend == "live" else "llm_tool_trace"
    write_outputs(rows, args.out, stem)
    return 0


if __name__ == "__main__":
    sys.exit(main())
