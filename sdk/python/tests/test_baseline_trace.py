from __future__ import annotations

import importlib.util
import os
import sys
from pathlib import Path

import pytest

from afp_sdk import AFPSidecarClient
from afp_sdk.testing import start_sidecar, stop_sidecar

pytestmark = pytest.mark.keep_afp_singleton

_PATH = Path(__file__).resolve().parents[1] / "examples" / "baseline_trace.py"
_SPEC = importlib.util.spec_from_file_location("baseline_trace", _PATH)
assert _SPEC is not None and _SPEC.loader is not None
_mod = importlib.util.module_from_spec(_SPEC)
sys.modules["baseline_trace"] = _mod
_SPEC.loader.exec_module(_mod)


@pytest.fixture
def sidecar() -> str:
    socket_path = f"/tmp/afp-base-test-{os.getpid()}-{id(object())}.sock"
    proc = start_sidecar(socket_path)
    yield socket_path
    AFPSidecarClient.reset_singleton()
    stop_sidecar(proc, socket_path)


def test_benign_matches_across_gates(sidecar: str) -> None:
    import llm_tool_trace as traces

    with traces.fetch_server() as base_url:
        rows = {
            gate: _mod.run_job_cell("s1_short", gate, sidecar, base_url)
            for gate in _mod.GATES
        }
    for gate, row in rows.items():
        assert row.task_completed, gate
        assert row.isolated is False, gate
        assert row.false_isolated is False, gate


def test_runaway_contained_only_with_a_gate(sidecar: str) -> None:
    import llm_tool_trace as traces

    with traces.fetch_server() as base_url:
        off = _mod.run_job_cell("s3_runaway", "off", sidecar, base_url)
        on = _mod.run_job_cell("s3_runaway", "afp", sidecar, base_url)
    assert off.isolated is False
    assert off.task_completed is True
    assert on.isolated is True
    assert on.task_completed is False


def test_stale_probe_splits_stateless_from_afp() -> None:
    suffix = f"{os.getpid()}-{id(object())}"
    sock_stateless = f"/tmp/afp-base-stale-s-{suffix}.sock"
    sock_afp = f"/tmp/afp-base-stale-a-{suffix}.sock"
    proc_s = start_sidecar(sock_stateless)
    proc_a = start_sidecar(sock_afp)
    try:
        stateless = {
            row.job: row for row in _mod.run_persist_pair("stateless", sock_stateless)
        }
        afp = {row.job: row for row in _mod.run_persist_pair("afp", sock_afp)}
    finally:
        AFPSidecarClient.reset_singleton()
        stop_sidecar(proc_s, sock_stateless)
        stop_sidecar(proc_a, sock_afp)

    assert stateless["stale_probe"].isolated is False
    assert afp["stale_probe"].isolated is True
    assert "recursion" in afp["stale_probe"].block_reason.lower()
    assert stateless["fresh_s1"].task_completed is True
    assert afp["fresh_s1"].task_completed is True
