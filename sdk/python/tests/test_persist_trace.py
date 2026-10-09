from __future__ import annotations

import importlib.util
import os
import sys
from pathlib import Path

import pytest

from afp_sdk import AFPSidecarClient
from afp_sdk.testing import start_sidecar, stop_sidecar

pytestmark = pytest.mark.keep_afp_singleton

_PATH = Path(__file__).resolve().parents[1] / "examples" / "persist_trace.py"
_SPEC = importlib.util.spec_from_file_location("persist_trace", _PATH)
assert _SPEC is not None and _SPEC.loader is not None
_mod = importlib.util.module_from_spec(_SPEC)
sys.modules["persist_trace"] = _mod
_SPEC.loader.exec_module(_mod)


@pytest.fixture
def persist_sidecar() -> str:
    socket_path = f"/tmp/afp-persist-test-{os.getpid()}-{id(object())}.sock"
    proc = start_sidecar(socket_path)
    os.environ["AFP_IPC_SOCKET"] = socket_path
    os.environ["AFP_SDK_FAIL_MODE"] = "closed"
    AFPSidecarClient.reset_singleton()
    yield socket_path
    AFPSidecarClient.reset_singleton()
    stop_sidecar(proc, socket_path)


def test_stale_probe_holds_and_fresh_s1_overwrites(persist_sidecar: str) -> None:
    rows = {row.arm: row for row in _mod.run_persist(persist_sidecar)}
    stale = rows["stale_probe"]
    fresh = rows["fresh_s1"]
    assert stale.isolated is True
    assert "recursion" in stale.block_reason.lower()
    assert fresh.isolated is False
    assert fresh.task_completed is True
