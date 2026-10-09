"""Helpers for spawning a local sidecar in examples and tests."""

from __future__ import annotations

import os
import socket
import subprocess
import time
from pathlib import Path
from typing import Mapping

REPO_ROOT = Path(__file__).resolve().parents[3]


def pick_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def public_reason(reason: str) -> str:
    text = reason.strip()
    lower = text.lower()
    for prefix in ("afp-core: ", "afp-core:"):
        if lower.startswith(prefix):
            return text[len(prefix) :].strip()
    return text


def start_sidecar(
    socket_path: str,
    *,
    extra_env: Mapping[str, str] | None = None,
    timeout_s: float = 25.0,
) -> subprocess.Popen[str]:
    if os.path.exists(socket_path):
        os.remove(socket_path)
    env = os.environ.copy()
    env.update(
        {
            "AFP_IPC_SOCKET": socket_path,
            "AFP_BOOTSTRAP_PATH": str(REPO_ROOT / "cmd/sidecar/bootstrap.json"),
            "AFP_INGRESS_ADDR": f":{pick_port()}",
            "AFP_EGRESS_ADDR": f"127.0.0.1:{pick_port()}",
            "AFP_METRICS_ADDR": f"127.0.0.1:{pick_port()}",
            "AFP_MAX_RECURSION_DEPTH": os.getenv("AFP_MAX_RECURSION_DEPTH", "10"),
            "AFP_SDK_FAIL_MODE": "closed",
        }
    )
    if extra_env:
        env.update(extra_env)
    proc = subprocess.Popen(
        ["go", "run", "./cmd/sidecar"],
        cwd=str(REPO_ROOT),
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        text=True,
    )
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        if proc.poll() is not None:
            err = proc.stderr.read() if proc.stderr else ""
            raise RuntimeError(f"sidecar exited early: {err}")
        if os.path.exists(socket_path):
            try:
                import grpc

                from afp_sdk.pb import sdk_ipc_pb2, sdk_ipc_pb2_grpc

                channel = grpc.insecure_channel(f"unix://{socket_path}")
                stub = sdk_ipc_pb2_grpc.AFPSidecarIPCStub(channel)
                stub.ReportInternalState(sdk_ipc_pb2.InternalStateReport(), timeout=0.3)
                channel.close()
                return proc
            except Exception:
                time.sleep(0.05)
                continue
        time.sleep(0.05)
    proc.terminate()
    raise TimeoutError(f"sidecar socket not ready: {socket_path}")


def stop_sidecar(proc: subprocess.Popen[str] | None, socket_path: str | None = None) -> None:
    if proc is not None:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
    if socket_path and os.path.exists(socket_path):
        os.remove(socket_path)
