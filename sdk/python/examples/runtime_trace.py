"""Paired LangGraph traces: CPL on vs off. No LLM weights required.

Same planner graph, two modes:
  off  — node body always runs (ungoverned stacking)
  on   — Path A pre-flight via sidecar before each hop

Run (from repo root, sidecar auto-spawned):
  PYTHONPATH=sdk/python python sdk/python/examples/runtime_trace.py \\
    --out artifacts/neurips-aiwild-2026/results
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import subprocess
import sys
import time
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

from langgraph.graph import END, StateGraph

from afp_sdk import AFPSidecarClient
from afp_sdk.exceptions import AFPQuotaExceededError
from afp_sdk.langgraph import afp_governed_node
from afp_sdk.testing import REPO_ROOT, public_reason, start_sidecar, stop_sidecar


@dataclass
class TraceResult:
    scenario: str
    mode: str
    task_completed: bool
    planner_steps: int
    preflight_probes: int
    isolated: bool
    block_reason: str
    final_depth: int
    wall_ms: float
    graph_stop_depth: int


def _planner_body(state: dict[str, Any]) -> dict[str, Any]:
    depth = int(state.get("recursion_depth", 0)) + 1
    messages = list(state.get("messages", []))
    messages.append(f"planned-step-{depth}")
    return {
        **state,
        "recursion_depth": depth,
        "messages": messages,
        "context_memory_bytes": max(64, sum(len(m) for m in messages) * 1024),
        "planner_steps": int(state.get("planner_steps", 0)) + 1,
    }


def build_app(*, governed: bool, graph_stop_depth: int, stats: dict[str, int]):
    if governed:
        gated = afp_governed_node(_planner_body, on_quota_exceeded="annotate")

        def planner(state: dict[str, Any]) -> dict[str, Any]:
            stats["preflight_probes"] += 1
            return gated(state)

    else:

        def planner(state: dict[str, Any]) -> dict[str, Any]:
            return _planner_body(state)

    graph = StateGraph(dict)
    graph.add_node("planner", planner)
    graph.set_entry_point("planner")

    def should_continue(state: dict[str, Any]) -> str:
        if state.get("afp_blocked"):
            return "stop"
        if int(state.get("recursion_depth", 0)) >= graph_stop_depth:
            return "stop"
        return "planner"

    graph.add_conditional_edges(
        "planner", should_continue, {"planner": "planner", "stop": END}
    )
    return graph.compile()


def run_once(
    *,
    scenario: str,
    governed: bool,
    graph_stop_depth: int,
    initial_depth: int,
    recursion_limit: int,
) -> TraceResult:
    AFPSidecarClient.reset_singleton()
    stats = {"preflight_probes": 0}
    app = build_app(
        governed=governed, graph_stop_depth=graph_stop_depth, stats=stats
    )
    state: dict[str, Any] = {
        "recursion_depth": initial_depth,
        "messages": ["seed"],
        "context_memory_bytes": 64,
        "planner_steps": 0,
        "trace_id": f"{scenario}-{'on' if governed else 'off'}",
    }

    started = time.perf_counter()
    try:
        final = app.invoke(state, config={"recursion_limit": recursion_limit})
    except AFPQuotaExceededError as exc:
        wall_ms = (time.perf_counter() - started) * 1000.0
        return TraceResult(
            scenario=scenario,
            mode="on" if governed else "off",
            task_completed=False,
            planner_steps=int(state.get("planner_steps", 0)),
            preflight_probes=stats["preflight_probes"],
            isolated=True,
            block_reason=str(exc),
            final_depth=int(state.get("recursion_depth", 0)),
            wall_ms=round(wall_ms, 2),
            graph_stop_depth=graph_stop_depth,
        )
    wall_ms = (time.perf_counter() - started) * 1000.0

    blocked = bool(final.get("afp_blocked"))
    steps = int(final.get("planner_steps", 0))
    depth = int(final.get("recursion_depth", 0))
    completed = (not blocked) and depth >= graph_stop_depth
    if scenario == "benign":
        completed = (not blocked) and depth >= graph_stop_depth

    return TraceResult(
        scenario=scenario,
        mode="on" if governed else "off",
        task_completed=completed,
        planner_steps=steps,
        preflight_probes=stats["preflight_probes"],
        isolated=blocked,
        block_reason=str(final.get("afp_block_reason") or ""),
        final_depth=depth,
        wall_ms=round(wall_ms, 2),
        graph_stop_depth=graph_stop_depth,
    )


def _public_reason(reason: str) -> str:
    return public_reason(reason)


def _row_dict(row: TraceResult) -> dict[str, Any]:
    data = asdict(row)
    data["block_reason"] = public_reason(str(data.get("block_reason") or ""))
    return data


def write_outputs(rows: list[TraceResult], out_dir: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    csv_path = out_dir / "runtime_trace.csv"
    json_path = out_dir / "runtime_trace.json"
    fieldnames = list(asdict(rows[0]).keys())
    with csv_path.open("w", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(_row_dict(row))
    json_path.write_text(json.dumps([_row_dict(r) for r in rows], indent=2) + "\n")
    print(f"wrote {csv_path}")
    print(f"wrote {json_path}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="CPL on/off LangGraph traces")
    parser.add_argument(
        "--out",
        type=Path,
        default=REPO_ROOT / "artifacts/neurips-aiwild-2026/results",
    )
    parser.add_argument(
        "--socket",
        default="",
        help="Existing AFP_IPC_SOCKET; if empty, spawn a sidecar",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    proc: subprocess.Popen[str] | None = None
    socket_path = args.socket or f"/tmp/afp-runtime-trace-{os.getpid()}.sock"

    try:
        if not args.socket:
            proc = start_sidecar(socket_path)
        os.environ["AFP_IPC_SOCKET"] = socket_path
        os.environ["AFP_SDK_FAIL_MODE"] = "closed"

        rows: list[TraceResult] = []
        # Benign: short graph, both modes should finish.
        for governed in (False, True):
            rows.append(
                run_once(
                    scenario="benign",
                    governed=governed,
                    graph_stop_depth=3,
                    initial_depth=0,
                    recursion_limit=20,
                )
            )
        # Runaway stacking: ungoverned hits the graph cap; governed should isolate first.
        for governed in (False, True):
            rows.append(
                run_once(
                    scenario="runaway",
                    governed=governed,
                    graph_stop_depth=40,
                    initial_depth=0,
                    recursion_limit=50,
                )
            )

        write_outputs(rows, args.out)
        for row in rows:
            print(
                f"{row.scenario:8} mode={row.mode:3} completed={row.task_completed} "
                f"steps={row.planner_steps:2} probes={row.preflight_probes:2} "
                f"isolated={row.isolated} depth={row.final_depth} "
                f"reason={row.block_reason[:70]}"
            )
        return 0
    finally:
        AFPSidecarClient.reset_singleton()
        stop_sidecar(proc, None if args.socket else socket_path)


if __name__ == "__main__":
    sys.exit(main())
