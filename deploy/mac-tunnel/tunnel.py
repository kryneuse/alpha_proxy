#!/usr/bin/env python3
"""Keep the alpha4 reverse SSH tunnel running; contains no credentials."""
import fcntl
import json
import os
from pathlib import Path
import signal
import shlex
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[2] / ".runtime" / "alpha4-pii-tunnel"
ROOT.mkdir(parents=True, exist_ok=True)
PID_FILE = ROOT / "supervisor.pid"
# The server also expires the session if the Mac disappears. Client-only
# keepalives cannot release a remote listener stuck on a half-open connection.
REMOTE_WATCHDOG = """import os, select, sys
while True:
    readable, _, _ = select.select([sys.stdin], [], [], 45)
    if not readable or not os.read(sys.stdin.fileno(), 4096):
        break
"""
SSH_COMMAND = [
    "/usr/bin/ssh", "-T",
    "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
    "-o", "ForwardAgent=no", "-o", "ControlMaster=no",
    "-o", "ControlPath=none", "-o", "ConnectTimeout=10",
    "-o", "ExitOnForwardFailure=yes", "-o", "ServerAliveInterval=15",
    "-o", "ServerAliveCountMax=3",
    "-R", "127.0.0.1:18081:127.0.0.1:8080", "alpha4",
    "python3 -u -c " + shlex.quote(REMOTE_WATCHDOG),
]
AUTOSSH_COMMAND = ["/opt/homebrew/bin/autossh", "-M", "0", *SSH_COMMAND[1:]]


def active_pid():
    try:
        pid = int(PID_FILE.read_text().strip())
        command = subprocess.check_output(
            ["/bin/ps", "-p", str(pid), "-o", "command="], text=True
        ).strip()
        if str(Path(__file__).resolve()) in command and command.endswith(" run"):
            return pid
    except (OSError, ValueError, subprocess.CalledProcessError):
        pass
    return None


def run():
    lock = (ROOT / "supervisor.lock").open("w")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        raise SystemExit("Tunnel supervisor already running")
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopped.set())
    PID_FILE.write_text(str(os.getpid()) + "\n")
    child = None
    delay = 1
    try:
        while not stopped.is_set():
            started = time.monotonic()
            env = os.environ.copy()
            env.update(AUTOSSH_PATH="/usr/bin/ssh", AUTOSSH_GATETIME="0",
                       AUTOSSH_POLL="10", AUTOSSH_FIRST_POLL="10",
                       AUTOSSH_LOGFILE=str(ROOT / "autossh.log"),
                       AUTOSSH_LOGLEVEL="6")
            child = subprocess.Popen(AUTOSSH_COMMAND, stdin=subprocess.PIPE, env=env)
            (ROOT / "state.json").write_text(json.dumps({
                "supervisor_pid": os.getpid(), "autossh_pid": child.pid,
                "public_url": "http://91.218.244.173:18080",
                "local_url": "http://127.0.0.1:8080",
                "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            }, indent=2) + "\n")
            print(f"{time.asctime()} autossh started pid={child.pid}", flush=True)
            last_heartbeat = 0.0
            while child.poll() is None and not stopped.wait(0.5):
                if time.monotonic() - last_heartbeat >= 5:
                    try:
                        child.stdin.write(b"heartbeat\n")
                        child.stdin.flush()
                    except (BrokenPipeError, OSError):
                        child.terminate()
                        break
                    last_heartbeat = time.monotonic()
            if child.poll() is None and not stopped.is_set():
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()
            try:
                child.stdin.close()
            except (BrokenPipeError, OSError):
                pass
            if stopped.is_set():
                break
            elapsed = time.monotonic() - started
            delay = 1 if elapsed >= 60 else min(delay * 2, 30)
            print(f"{time.asctime()} autossh exited={child.returncode}; restart in {delay}s", flush=True)
            stopped.wait(delay)
    finally:
        if child is not None and child.poll() is None:
            child.terminate()
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait()
        PID_FILE.unlink(missing_ok=True)
        (ROOT / "state.json").unlink(missing_ok=True)


def main():
    action = sys.argv[1] if len(sys.argv) > 1 else "status"
    if action == "run":
        run()
        return
    pid = active_pid()
    if action == "start":
        if pid:
            print(f"Already running: {pid}")
            return
        with (ROOT / "supervisor.log").open("a") as log:
            child = subprocess.Popen(
                [sys.executable, str(Path(__file__).resolve()), "run"],
                stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                start_new_session=True,
            )
        time.sleep(1)
        if child.poll() is not None:
            raise SystemExit("Supervisor exited; inspect supervisor.log")
        print(f"Started supervisor: {child.pid}")
    elif action == "stop":
        if pid:
            os.kill(pid, signal.SIGTERM)
            print(f"Stopping supervisor: {pid}")
        else:
            print("Not running")
    elif action == "status":
        print(f"Supervisor running: {pid}" if pid else "Not running")
        if pid and (ROOT / "state.json").exists():
            print((ROOT / "state.json").read_text())
    else:
        raise SystemExit("Usage: tunnel.py start|stop|status")


if __name__ == "__main__":
    main()
