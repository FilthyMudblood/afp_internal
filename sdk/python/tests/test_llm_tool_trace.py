from __future__ import annotations

import importlib.util
import os
import sys
from pathlib import Path

import pytest

from afp_sdk import AFPSidecarClient
from afp_sdk.testing import start_sidecar, stop_sidecar

pytestmark = pytest.mark.keep_afp_singleton

_PATH = Path(__file__).resolve().parents[1] / "examples" / "llm_tool_trace.py"
_SPEC = importlib.util.spec_from_file_location("llm_tool_trace", _PATH)
assert _SPEC is not None and _SPEC.loader is not None
_mod = importlib.util.module_from_spec(_SPEC)
sys.modules["llm_tool_trace"] = _mod
_SPEC.loader.exec_module(_mod)


@pytest.fixture
def fresh_sidecar() -> str:
    socket_path = f"/tmp/afp-llm-tool-test-{os.getpid()}-{id(object())}.sock"
    proc = start_sidecar(socket_path)
    os.environ["AFP_IPC_SOCKET"] = socket_path
    os.environ["AFP_SDK_FAIL_MODE"] = "closed"
    AFPSidecarClient.reset_singleton()
    yield socket_path
    AFPSidecarClient.reset_singleton()
    stop_sidecar(proc, socket_path)


def _pair(job_name: str, sidecar: str, monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setenv("AFP_IPC_SOCKET", sidecar)
    monkeypatch.setenv("AFP_SDK_FAIL_MODE", "closed")
    AFPSidecarClient.reset_singleton()
    job = _mod.JOBS[job_name]
    with _mod.fetch_server() as base_url:
        off = _mod.run_once(
            job=job, governed=False, backend="scripted", seed=0, base_url=base_url
        )
        on = _mod.run_once(
            job=job, governed=True, backend="scripted", seed=0, base_url=base_url
        )
    return off, on


def test_s1_short_completes_both_modes(fresh_sidecar: str, monkeypatch: pytest.MonkeyPatch) -> None:
    off, on = _pair("s1_short", fresh_sidecar, monkeypatch)
    assert off.task_completed and on.task_completed
    assert off.isolated is False and on.isolated is False
    assert off.false_isolated is False and on.false_isolated is False
    assert off.tool_calls == 1 and on.tool_calls == 1
    assert on.preflight_probes >= 2


def test_s2_long_benign_no_false_isolate(
    fresh_sidecar: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    off, on = _pair("s2_long", fresh_sidecar, monkeypatch)
    assert off.task_completed and on.task_completed
    assert on.false_isolated is False
    assert off.tool_calls == 8 and on.tool_calls == 8
    assert on.final_depth <= 10


def test_s3_runaway_isolated_only_with_cpl(
    fresh_sidecar: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    off, on = _pair("s3_runaway", fresh_sidecar, monkeypatch)
    assert off.isolated is False
    assert off.task_completed
    assert off.final_depth >= 40
    assert on.isolated is True
    assert on.task_completed is False
    assert on.planner_steps < off.planner_steps
    assert "recursion" in on.block_reason.lower()


def test_s4_storm_isolated_on_burst(
    fresh_sidecar: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    off, on = _pair("s4_storm", fresh_sidecar, monkeypatch)
    assert off.isolated is False
    assert off.task_completed
    assert off.tool_calls == 1 + _mod.STORM_N
    assert on.isolated is True
    assert on.task_completed is False
    assert on.tool_calls == 0
    assert on.false_isolated is False


def test_public_reason_strips_prefix() -> None:
    assert _mod.public_reason("afp-core: entropy 0.96") == "entropy 0.96"


def test_live_s1_uses_openai_compatible_tool_call(
    fresh_sidecar: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AFP_IPC_SOCKET", fresh_sidecar)
    monkeypatch.setenv("AFP_SDK_FAIL_MODE", "closed")
    AFPSidecarClient.reset_singleton()
    calls = {"n": 0}

    def fake_chat(messages):
        calls["n"] += 1
        if calls["n"] == 1:
            return {
                "tool_calls": [
                    {
                        "function": {
                            "name": "lookup",
                            "arguments": '{"key": "capital:france"}',
                        }
                    }
                ]
            }, 16
        return {"content": "Paris"}, 8

    monkeypatch.setattr(_mod, "_openai_chat", fake_chat)
    job = _mod.JOBS["s1_short"]
    with _mod.fetch_server() as base_url:
        row = _mod.run_once(
            job=job, governed=True, backend="live", seed=0, base_url=base_url
        )
    assert row.backend == "live"
    assert row.task_completed
    assert row.tool_calls == 1
    assert row.prompt_tokens == 24
    assert row.false_isolated is False
