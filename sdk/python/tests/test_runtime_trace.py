from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

import pytest

from afp_sdk import AFPSidecarClient

pytestmark = pytest.mark.keep_afp_singleton

_PATH = Path(__file__).resolve().parents[1] / "examples" / "runtime_trace.py"
_SPEC = importlib.util.spec_from_file_location("runtime_trace", _PATH)
assert _SPEC is not None and _SPEC.loader is not None
_trace = importlib.util.module_from_spec(_SPEC)
sys.modules["runtime_trace"] = _trace
_SPEC.loader.exec_module(_trace)


def test_benign_completes_with_and_without_cpl(sidecar_socket: str, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AFP_IPC_SOCKET", sidecar_socket)
    monkeypatch.setenv("AFP_SDK_FAIL_MODE", "closed")
    AFPSidecarClient.reset_singleton()

    off = _trace.run_once(
        scenario="benign",
        governed=False,
        graph_stop_depth=3,
        initial_depth=0,
        recursion_limit=20,
    )
    on = _trace.run_once(
        scenario="benign",
        governed=True,
        graph_stop_depth=3,
        initial_depth=0,
        recursion_limit=20,
    )
    assert off.task_completed and on.task_completed
    assert off.isolated is False and on.isolated is False
    assert off.planner_steps == 3 and on.planner_steps == 3
    assert on.preflight_probes == 3


def test_runaway_isolated_only_with_cpl(sidecar_socket: str, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AFP_IPC_SOCKET", sidecar_socket)
    monkeypatch.setenv("AFP_SDK_FAIL_MODE", "closed")
    AFPSidecarClient.reset_singleton()

    off = _trace.run_once(
        scenario="runaway",
        governed=False,
        graph_stop_depth=40,
        initial_depth=0,
        recursion_limit=50,
    )
    on = _trace.run_once(
        scenario="runaway",
        governed=True,
        graph_stop_depth=40,
        initial_depth=0,
        recursion_limit=50,
    )
    assert off.isolated is False
    assert off.planner_steps == 40
    assert on.isolated is True
    assert on.planner_steps < off.planner_steps
    assert "recursion" in on.block_reason.lower()


def test_public_reason_strips_implementation_prefix() -> None:
    assert _trace._public_reason("afp-core: recursion depth exceeded") == "recursion depth exceeded"
    assert _trace._public_reason("recursion depth exceeded") == "recursion depth exceeded"
