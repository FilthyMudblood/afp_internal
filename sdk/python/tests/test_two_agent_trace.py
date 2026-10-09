from __future__ import annotations

import importlib.util
import os
import sys
from pathlib import Path

import pytest

from afp_sdk import AFPSidecarClient
from afp_sdk.testing import start_sidecar, stop_sidecar

pytestmark = pytest.mark.keep_afp_singleton

_PATH = Path(__file__).resolve().parents[1] / "examples" / "two_agent_trace.py"
_SPEC = importlib.util.spec_from_file_location("two_agent_trace", _PATH)
assert _SPEC is not None and _SPEC.loader is not None
_mod = importlib.util.module_from_spec(_SPEC)
sys.modules["two_agent_trace"] = _mod
_SPEC.loader.exec_module(_mod)


@pytest.fixture
def pair_sidecars() -> tuple[str, str]:
    suffix = f"{os.getpid()}-{id(object())}"
    sock_a = f"/tmp/afp-two-test-a-{suffix}.sock"
    sock_b = f"/tmp/afp-two-test-b-{suffix}.sock"
    proc_a = start_sidecar(sock_a)
    proc_b = start_sidecar(sock_b)
    yield sock_a, sock_b
    AFPSidecarClient.reset_singleton()
    stop_sidecar(proc_a, sock_a)
    stop_sidecar(proc_b, sock_b)


@pytest.fixture
def shared_sidecar() -> str:
    socket_path = f"/tmp/afp-two-test-shared-{os.getpid()}-{id(object())}.sock"
    proc = start_sidecar(socket_path)
    yield socket_path
    AFPSidecarClient.reset_singleton()
    stop_sidecar(proc, socket_path)


def test_split_sidecars_b_still_completes(pair_sidecars: tuple[str, str]) -> None:
    sock_a, sock_b = pair_sidecars
    rows = {row.agent: row for row in _mod.run_split(sock_a, sock_b)}
    assert rows["did:afp:agent:a"].isolated is True
    assert rows["did:afp:agent:a"].task_completed is False
    assert rows["did:afp:agent:b"].isolated is False
    assert rows["did:afp:agent:b"].task_completed is True
    assert rows["did:afp:agent:b"].tools == 1


def test_shared_sidecar_b_overwrites_host_telemetry(shared_sidecar: str) -> None:
    rows = {row.agent: row for row in _mod.run_shared(shared_sidecar)}
    assert rows["did:afp:agent:a"].isolated is True
    assert rows["did:afp:agent:b"].isolated is False
    assert rows["did:afp:agent:b"].task_completed is True
