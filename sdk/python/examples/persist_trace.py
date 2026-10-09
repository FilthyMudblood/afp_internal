"""Cross-request persistence on one Path A sidecar.

After S3 isolates, a new LangGraph (fresh SDK client + fresh graph state)
talks to the same sidecar. The kernel stores last-reported recursion depth
on the host monitor, not a per-request letter. Two arms:

  stale_probe — preflight without rewriting telemetry (leftover depth)
  fresh_s1    — new S1 job reports its own depth starting at 0
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

from afp_sdk import AFPSidecarClient
from afp_sdk.exceptions import AFPQuotaExceededError
from afp_sdk.testing import REPO_ROOT, public_reason, start_sidecar, stop_sidecar

_EXAMPLES = Path(__file__).resolve().parent
if str(_EXAMPLES) not in sys.path:
    sys.path.insert(0, str(_EXAMPLES))

import llm_tool_trace as traces  # noqa: E402


@dataclass
class PersistRow:
    experiment: str
    arm: str
    isolated: bool
    task_completed: bool
    probes: int
    block_reason: str
    leftover_depth: int
    note: str


def _bind_socket(socket_path: str) -> None:
    os.environ["AFP_IPC_SOCKET"] = socket_path
    os.environ["AFP_SDK_FAIL_MODE"] = "closed"
    AFPSidecarClient.reset_singleton()


def reset_telemetry() -> None:
    """Forget leftover host depth so the next probe is per-step only."""
    AFPSidecarClient.reset_singleton()
    client = AFPSidecarClient()
    client.report_state(current_recursion_depth=0, context_memory_bytes=64)


def isolate_runaway(base_url: str) -> traces.ToolTraceResult:
    job = traces.JOBS["s3_runaway"]
    row = traces.run_once(
        job=job, governed=True, backend="scripted", seed=0, base_url=base_url
    )
    if not row.isolated:
        raise RuntimeError(f"S3 did not isolate: {asdict(row)}")
    return row


def stale_probe() -> PersistRow:
    """New client, no ReportInternalState — leftover depth should still isolate."""
    AFPSidecarClient.reset_singleton()
    client = AFPSidecarClient()
    isolated = False
    reason = ""
    try:
        client.pre_flight_check(estimated_tasks=1, trace_id="persist-stale")
    except AFPQuotaExceededError as exc:
        isolated = True
        reason = public_reason(str(exc))
    return PersistRow(
        experiment="persist",
        arm="stale_probe",
        isolated=isolated,
        task_completed=False,
        probes=1,
        block_reason=reason,
        leftover_depth=11,
        note="preflight without rewriting sidecar telemetry",
    )


def fresh_s1(base_url: str) -> PersistRow:
    """New graph reports depth 0 — measures whether the isolate letter survived."""
    AFPSidecarClient.reset_singleton()
    row = traces.run_once(
        job=traces.JOBS["s1_short"],
        governed=True,
        backend="scripted",
        seed=0,
        base_url=base_url,
    )
    return PersistRow(
        experiment="persist",
        arm="fresh_s1",
        isolated=row.isolated,
        task_completed=row.task_completed,
        probes=row.preflight_probes,
        block_reason=row.block_reason,
        leftover_depth=0,
        note="new S1 reports its own depth starting at 0",
    )


def run_persist(socket_path: str) -> list[PersistRow]:
    _bind_socket(socket_path)
    rows: list[PersistRow] = []
    with traces.fetch_server() as base_url:
        isolate_runaway(base_url)
        rows.append(stale_probe())
        # leftover depth is still 11; now measure a graph that overwrites it
        rows.append(fresh_s1(base_url))
    return rows


def write_outputs(rows: list[Any], out_dir: Path, stem: str) -> None:
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
    parser = argparse.ArgumentParser(description="Path A cross-request persistence")
    parser.add_argument(
        "--out",
        type=Path,
        default=REPO_ROOT / "artifacts/neurips-aiwild-2026/results",
    )
    parser.add_argument("--socket", default="")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    proc = None
    socket_path = args.socket or f"/tmp/afp-persist-{os.getpid()}.sock"
    try:
        if not args.socket:
            proc = start_sidecar(socket_path)
        rows = run_persist(socket_path)
        for row in rows:
            print(
                f"{row.arm:12} isolated={row.isolated} "
                f"completed={row.task_completed} reason={row.block_reason[:70]}"
            )
        write_outputs(rows, args.out, "persist_trace")
    finally:
        AFPSidecarClient.reset_singleton()
        if not args.socket:
            stop_sidecar(proc, socket_path)
    return 0


if __name__ == "__main__":
    sys.exit(main())
