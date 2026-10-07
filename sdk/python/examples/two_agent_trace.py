"""Two local Path A planners. SEA is host-scoped (one sidecar = one monitor).

  split_sidecars — agent A isolates on sidecar A; agent B runs S1 on sidecar B
  shared_sidecar — both planners share one sidecar (A isolates, then B runs S1)
"""

from __future__ import annotations

import argparse
import os
import sys
from dataclasses import dataclass
from pathlib import Path

from afp_sdk import AFPSidecarClient
from afp_sdk.testing import REPO_ROOT, start_sidecar, stop_sidecar

_EXAMPLES = Path(__file__).resolve().parent
if str(_EXAMPLES) not in sys.path:
    sys.path.insert(0, str(_EXAMPLES))

import llm_tool_trace as traces  # noqa: E402
import persist_trace as persist  # noqa: E402


@dataclass
class TwoAgentRow:
    experiment: str
    layout: str
    agent: str
    job: str
    isolated: bool
    task_completed: bool
    tools: int
    probes: int
    block_reason: str


def _bind_socket(socket_path: str) -> None:
    os.environ["AFP_IPC_SOCKET"] = socket_path
    os.environ["AFP_SDK_FAIL_MODE"] = "closed"
    AFPSidecarClient.reset_singleton()


def _row(layout: str, agent: str, job: str, result: traces.ToolTraceResult) -> TwoAgentRow:
    return TwoAgentRow(
        experiment="two_agent",
        layout=layout,
        agent=agent,
        job=job,
        isolated=result.isolated,
        task_completed=result.task_completed,
        tools=result.tool_calls,
        probes=result.preflight_probes,
        block_reason=result.block_reason,
    )


def run_split(socket_a: str, socket_b: str) -> list[TwoAgentRow]:
    rows: list[TwoAgentRow] = []
    with traces.fetch_server() as base_url:
        _bind_socket(socket_a)
        a = traces.run_once(
            job=traces.JOBS["s3_runaway"],
            governed=True,
            backend="scripted",
            seed=0,
            base_url=base_url,
        )
        rows.append(_row("split_sidecars", "did:afp:agent:a", "s3_runaway", a))

        _bind_socket(socket_b)
        b = traces.run_once(
            job=traces.JOBS["s1_short"],
            governed=True,
            backend="scripted",
            seed=0,
            base_url=base_url,
        )
        rows.append(_row("split_sidecars", "did:afp:agent:b", "s1_short", b))
    return rows


def run_shared(socket_path: str) -> list[TwoAgentRow]:
    rows: list[TwoAgentRow] = []
    with traces.fetch_server() as base_url:
        _bind_socket(socket_path)
        a = traces.run_once(
            job=traces.JOBS["s3_runaway"],
            governed=True,
            backend="scripted",
            seed=0,
            base_url=base_url,
        )
        rows.append(_row("shared_sidecar", "did:afp:agent:a", "s3_runaway", a))
        b = traces.run_once(
            job=traces.JOBS["s1_short"],
            governed=True,
            backend="scripted",
            seed=0,
            base_url=base_url,
        )
        rows.append(_row("shared_sidecar", "did:afp:agent:b", "s1_short", b))
    return rows


def write_outputs(rows: list[TwoAgentRow], out_dir: Path) -> None:
    persist.write_outputs(rows, out_dir, "two_agent_trace")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Two local Path A planners")
    parser.add_argument(
        "--out",
        type=Path,
        default=REPO_ROOT / "artifacts/neurips-aiwild-2026/results",
    )
    parser.add_argument(
        "--layout",
        choices=("split", "shared", "both"),
        default="both",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    rows: list[TwoAgentRow] = []
    procs: list[tuple[object, str]] = []
    try:
        if args.layout in {"split", "both"}:
            sock_a = f"/tmp/afp-two-a-{os.getpid()}.sock"
            sock_b = f"/tmp/afp-two-b-{os.getpid()}.sock"
            procs.append((start_sidecar(sock_a), sock_a))
            procs.append((start_sidecar(sock_b), sock_b))
            rows.extend(run_split(sock_a, sock_b))
        if args.layout in {"shared", "both"}:
            sock = f"/tmp/afp-two-shared-{os.getpid()}.sock"
            procs.append((start_sidecar(sock), sock))
            rows.extend(run_shared(sock))
        for row in rows:
            print(
                f"{row.layout:16} {row.agent:18} job={row.job:10} "
                f"isolated={row.isolated} completed={row.task_completed}"
            )
        write_outputs(rows, args.out)
    finally:
        AFPSidecarClient.reset_singleton()
        for proc, socket_path in procs:
            stop_sidecar(proc, socket_path)  # type: ignore[arg-type]
    return 0


if __name__ == "__main__":
    sys.exit(main())
