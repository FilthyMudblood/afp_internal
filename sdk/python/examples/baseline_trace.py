"""Three-way Path A baseline: no gate, stateless per-step, current sidecar.

On S1–S4 a fresh sidecar makes stateless and AFP look the same: both judge
the current hop. They split after isolate: AFP keeps leftover telemetry;
stateless reports depth 0 before the next probe.

  off        — no preflight
  stateless  — same sidecar checks, leftover wiped between requests
  afp        — same sidecar checks, leftover kept
"""

from __future__ import annotations

import argparse
import os
import sys
from dataclasses import asdict, dataclass
from pathlib import Path

from afp_sdk import AFPSidecarClient
from afp_sdk.testing import REPO_ROOT, start_sidecar, stop_sidecar

_EXAMPLES = Path(__file__).resolve().parent
if str(_EXAMPLES) not in sys.path:
    sys.path.insert(0, str(_EXAMPLES))

import llm_tool_trace as traces  # noqa: E402
import persist_trace as persist  # noqa: E402

GATES = ("off", "stateless", "afp")
BENIGN_JOBS = ("s1_short", "s2_long", "s2_lookups", "s2_fetches", "s2_repeat")
CONTAIN_JOBS = ("s3_runaway", "s4_storm")
JOB_NAMES = BENIGN_JOBS + CONTAIN_JOBS


@dataclass
class BaselineRow:
    experiment: str
    gate: str
    job: str
    task_completed: bool
    isolated: bool
    false_isolated: bool
    tools: int
    probes: int
    wall_ms: float
    block_reason: str
    note: str


def _bind_socket(socket_path: str) -> None:
    os.environ["AFP_IPC_SOCKET"] = socket_path
    os.environ["AFP_SDK_FAIL_MODE"] = "closed"
    AFPSidecarClient.reset_singleton()


def _from_job(gate: str, result: traces.ToolTraceResult, note: str) -> BaselineRow:
    return BaselineRow(
        experiment="jobs",
        gate=gate,
        job=result.job,
        task_completed=result.task_completed,
        isolated=result.isolated,
        false_isolated=result.false_isolated,
        tools=result.tool_calls,
        probes=result.preflight_probes,
        wall_ms=result.wall_ms,
        block_reason=result.block_reason,
        note=note,
    )


def run_job_cell(job_name: str, gate: str, socket_path: str, base_url: str) -> BaselineRow:
    _bind_socket(socket_path)
    if gate == "stateless":
        persist.reset_telemetry()
    result = traces.run_once(
        job=traces.JOBS[job_name],
        governed=gate != "off",
        backend="scripted",
        seed=0,
        base_url=base_url,
    )
    note = {
        "off": "no preflight",
        "stateless": "per-step check; leftover wiped at request start",
        "afp": "per-step check; leftover kept",
    }[gate]
    return _from_job(gate, result, note)


def run_jobs(socket_factory) -> list[BaselineRow]:
    rows: list[BaselineRow] = []
    with traces.fetch_server() as base_url:
        for job_name in JOB_NAMES:
            for gate in GATES:
                socket_path = socket_factory(job_name, gate)
                rows.append(run_job_cell(job_name, gate, socket_path, base_url))
    return rows


def run_persist_pair(gate: str, socket_path: str) -> list[BaselineRow]:
    persist._bind_socket(socket_path)
    rows: list[BaselineRow] = []
    with traces.fetch_server() as base_url:
        persist.isolate_runaway(base_url)
        if gate == "stateless":
            persist.reset_telemetry()
        stale = persist.stale_probe()
        rows.append(
            BaselineRow(
                experiment="persist",
                gate=gate,
                job="stale_probe",
                task_completed=False,
                isolated=stale.isolated,
                false_isolated=False,
                tools=0,
                probes=stale.probes,
                wall_ms=0.0,
                block_reason=stale.block_reason,
                note=(
                    "preflight after wipe"
                    if gate == "stateless"
                    else "preflight without rewriting leftover depth"
                ),
            )
        )
        fresh = persist.fresh_s1(base_url)
        rows.append(
            BaselineRow(
                experiment="persist",
                gate=gate,
                job="fresh_s1",
                task_completed=fresh.task_completed,
                isolated=fresh.isolated,
                false_isolated=False,
                tools=1 if fresh.task_completed else 0,
                probes=fresh.probes,
                wall_ms=0.0,
                block_reason=fresh.block_reason,
                note="new S1 reports depth 0",
            )
        )
    return rows


def summarize(rows: list[BaselineRow]) -> None:
    print("\nsummary")
    job_rows = [row for row in rows if row.experiment == "jobs"]
    for gate in GATES:
        benign = [row for row in job_rows if row.gate == gate and row.job in BENIGN_JOBS]
        contain = [row for row in job_rows if row.gate == gate and row.job in CONTAIN_JOBS]
        done = sum(1 for row in benign if row.task_completed)
        false_iso = sum(1 for row in benign if row.false_isolated)
        stopped = sum(1 for row in contain if row.isolated)
        mean_ms = (
            sum(row.wall_ms for row in benign) / len(benign) if benign else 0.0
        )
        print(
            f"  {gate:10} benign {done}/{len(benign)} "
            f"false_iso={false_iso} contained={stopped}/{len(contain)} "
            f"benign_mean_ms={mean_ms:.2f}"
        )
    persist_rows = [row for row in rows if row.experiment == "persist"]
    for row in persist_rows:
        print(
            f"  persist {row.gate:10} {row.job:12} "
            f"isolated={row.isolated} completed={row.task_completed}"
        )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="No gate / stateless / AFP baseline")
    parser.add_argument(
        "--out",
        type=Path,
        default=REPO_ROOT / "artifacts/neurips-aiwild-2026/results",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    rows: list[BaselineRow] = []
    procs: list[tuple[object, str]] = []
    pid = os.getpid()

    def socket_factory(job_name: str, gate: str) -> str:
        socket_path = f"/tmp/afp-base-{pid}-{job_name}-{gate}.sock"
        procs.append((start_sidecar(socket_path), socket_path))
        return socket_path

    try:
        rows.extend(run_jobs(socket_factory))
        for gate in ("stateless", "afp"):
            socket_path = f"/tmp/afp-base-{pid}-persist-{gate}.sock"
            procs.append((start_sidecar(socket_path), socket_path))
            rows.extend(run_persist_pair(gate, socket_path))
        for row in rows:
            print(
                f"{row.experiment:7} {row.gate:10} {row.job:12} "
                f"done={row.task_completed} isolated={row.isolated} "
                f"tools={row.tools:2} ms={row.wall_ms:.2f}"
            )
        summarize(rows)
        persist.write_outputs(rows, args.out, "baseline_trace")
    finally:
        AFPSidecarClient.reset_singleton()
        for proc, socket_path in procs:
            stop_sidecar(proc, socket_path)  # type: ignore[arg-type]
    return 0


if __name__ == "__main__":
    sys.exit(main())
