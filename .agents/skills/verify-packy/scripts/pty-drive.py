#!/usr/bin/env python3
"""Drive one Packy process in a private PTY, recording its raw terminal output."""
import argparse
import fcntl
import os
import pty
import select
import signal
import struct
import subprocess
import termios
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("binary")
parser.add_argument("transcript")
parser.add_argument("--step", action="append", nargs=2, metavar=("WAIT_FOR", "SEND"), required=True,
                    help="wait for literal terminal output, then send keys (ESC, TAB, ENTER, or literal text)")
args = parser.parse_args()
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 160, 0, 0))
process = None
output = bytearray()
try:
    process = subprocess.Popen([args.binary], stdin=slave, stdout=slave, stderr=slave,
                               start_new_session=True, close_fds=True, env=os.environ.copy())
    os.close(slave)
    slave = -1
    for expected, keys in args.step:
        seen = bytearray()
        deadline = time.monotonic() + 30
        while expected.encode() not in seen:
            if time.monotonic() >= deadline or process.poll() is not None:
                raise RuntimeError(f"did not observe {expected!r} before timeout or exit")
            ready, _, _ = select.select([master], [], [], min(0.2, deadline - time.monotonic()))
            if ready:
                chunk = os.read(master, 65536)
                output.extend(chunk)
                seen.extend(chunk)
        print(f"observed {expected!r}; sent {keys!r}", flush=True)
        os.write(master, {"ESC": b"\x1b", "TAB": b"\t", "ENTER": b"\r"}.get(keys, keys.encode()))
    deadline = time.monotonic() + 15
    while process.poll() is None and time.monotonic() < deadline:
        ready, _, _ = select.select([master], [], [], 0.2)
        if ready:
            try:
                output.extend(os.read(master, 65536))
            except OSError:
                break
    process.wait(timeout=max(0.1, deadline - time.monotonic()))
    if process.returncode != 0:
        raise RuntimeError(f"Packy exited {process.returncode}")
    print("Packy exited 0", flush=True)
finally:
    if process is not None:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait()
    if slave >= 0:
        os.close(slave)
    os.close(master)
    with open(args.transcript, "wb") as transcript:
        transcript.write(output)
