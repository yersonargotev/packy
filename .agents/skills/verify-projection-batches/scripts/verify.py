#!/usr/bin/env python3
"""Check the two-target model and replay its bounded inputs on Packy's executor."""

import argparse
from contextlib import ExitStack
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request


SKILL = Path(__file__).resolve().parents[1]
ROOT = SKILL.parents[2]
SOURCE = ROOT / "internal/localprojection/localprojection.go"
CONTROL = "^TestFormalProjectionBatch$/^present-3_replace-3_stale-0_fault-backup-2$"


def download(cache, name, asset):
    destination = cache / name
    if not destination.exists():
        print("Downloading " + name, flush=True)
        request = urllib.request.Request(asset["url"], headers={"User-Agent": "Packy-formal-verification"})
        with urllib.request.urlopen(request, timeout=60) as response:
            data = response.read()
        if hashlib.sha256(data).hexdigest() != asset["sha256"]:
            raise RuntimeError("download checksum mismatch: " + name)
        with tempfile.NamedTemporaryFile(dir=cache, delete=False) as temporary:
            temporary.write(data)
            temporary_path = Path(temporary.name)
        temporary_path.replace(destination)
    if hashlib.sha256(destination.read_bytes()).hexdigest() != asset["sha256"]:
        raise RuntimeError("cached tool checksum mismatch: " + str(destination))
    return destination


def tools():
    lock = json.loads((SKILL / "scripts/tools.json").read_text())
    cache = Path.home() / ".cache/packy-formal-verification"
    cache.mkdir(parents=True, exist_ok=True)
    jar = download(cache, "tla2tools-" + lock["tlc"]["version"] + ".jar", lock["tlc"])
    machine = {"arm64": "aarch64", "amd64": "x86_64"}.get(platform.machine().lower(), platform.machine().lower())
    system = platform.system().lower()
    key = system + "-" + machine
    if key not in lock["java"]["assets"]:
        raise RuntimeError("pinned Java runtime unavailable for " + key)
    name = "temurin-" + lock["java"]["version"] + "-" + system.replace("darwin", "mac") + "-" + machine
    archive = download(cache, name + ".tar.gz", lock["java"]["assets"][key])
    runtime = cache / name
    if not runtime.exists():
        with tempfile.TemporaryDirectory(dir=cache) as stage:
            # Only extract the exact upstream archive verified against tools.json.
            with tarfile.open(archive) as package:
                package.extractall(stage)
            Path(stage).rename(runtime)
    binaries = list(runtime.glob("**/bin/java"))
    if len(binaries) != 1:
        raise RuntimeError("expected exactly one Java executable under " + str(runtime))
    return binaries[0], jar


def environment(out):
    result = subprocess.run(["go", "env", "GOCACHE", "GOMODCACHE", "GOPATH"],
                            cwd=ROOT, check=True, text=True, capture_output=True, timeout=30)
    caches = result.stdout.splitlines()
    if len(caches) != 3:
        raise RuntimeError("go env did not return all three cache paths")
    env = os.environ.copy()
    for key, directory in (("HOME", "home"), ("XDG_CONFIG_HOME", "config"), ("TMPDIR", "tmp")):
        path = out / directory
        path.mkdir()
        env[key] = str(path)
    env.update(dict(zip(("GOCACHE", "GOMODCACHE", "GOPATH"), caches)))
    env.update(GOENV="off", GOTOOLCHAIN="local", GOWORK="off")
    return env


def run(command, cwd, env, log, error_log=None):
    with ExitStack() as files:
        stream = files.enter_context(log.open("w"))
        errors = files.enter_context(error_log.open("w")) if error_log else subprocess.STDOUT
        result = subprocess.run([str(arg) for arg in command], cwd=cwd, env=env,
                                stdout=stream, stderr=errors, timeout=180)
    return result.returncode


def model(out, env, java, jar, name, allow_stale=True, rollback=True):
    directory = out / name
    directory.mkdir()
    shutil.copy(SKILL / "models/ProjectionBatch.tla", directory)
    config = (SKILL / "models/ProjectionBatch.cfg").read_text()
    if not allow_stale:
        config = config.replace("AllowStale = TRUE", "AllowStale = FALSE")
    if not rollback:
        config = config.replace("RollbackOnBackupFailure = TRUE", "RollbackOnBackupFailure = FALSE")
    (directory / "ProjectionBatch.cfg").write_text(config)
    shutil.copy(SKILL / "models/view.mjs", directory)
    log = directory / "tlc.out"
    status = run([java, "-XX:+UseParallelGC", "-cp", jar, "tlc2.TLC", "-workers", "1",
                  "-fp", "0", "-lncheck", "final", "-config", "ProjectionBatch.cfg", "ProjectionBatch"], directory, env, log)
    output = log.read_text()
    if status == 12 and "Invariant RejectedBatchRestoresTargets is violated" in output:
        verdict = "violation"
    elif status == 0 and "Model checking completed. No error has been found." in output:
        verdict = "pass"
    else:
        raise RuntimeError("TLC did not finish checking the invariant; inspect " + str(log))
    print(name + ": " + verdict + " (" + str(log) + ")", flush=True)
    return verdict


def replay(out, env, name, pattern="^TestFormalProjectionBatch$", overlay=None):
    command = ["go", "test", "-json", "-count=1", "-tags=formalverification"]
    if overlay:
        command.extend(["-overlay", overlay])
    command.extend(["./internal/localprojection", "-run", pattern])
    raw = out / (name + ".jsonl")
    status = run(command, ROOT, env, raw, out / (name + ".stderr"))
    events = [json.loads(line) for line in raw.read_text().splitlines()]
    log = out / (name + ".out")
    log.write_text("".join(event.get("Output", "") for event in events))
    expected = {"TestFormalProjectionBatch/present-3_replace-3_stale-0_fault-backup-2"}
    if pattern != CONTROL:
        expected = {"TestFormalProjectionBatch/present-%d_replace-%d_stale-%d_fault-%s" % (p, r, s, f)
                    for p in range(4) for r in range(4) for s in range(4)
                    for f in ("none", "backup-1", "backup-2", "publish-1", "publish-2")}
    completed = [event for event in events if event.get("Action") in ("pass", "fail", "skip")
                 and event.get("Test", "").startswith("TestFormalProjectionBatch/")]
    if {event["Test"] for event in completed} != expected or len(completed) != len(expected):
        raise RuntimeError("Go replay did not complete exactly %d expected cases; inspect %s" % (len(expected), log))
    failures = 0
    for event in completed:
        if event["Action"] == "skip":
            raise RuntimeError("Go replay skipped a required case; inspect " + str(log))
        if event["Action"] != "fail":
            continue
        assertions = [match.group(1) for output in events if output.get("Test") == event["Test"]
                      for match in re.finditer(r"formal_batch_test\.go:\d+: (.*)", output.get("Output", ""))]
        if not assertions or any(not assertion.startswith("INVARIANT RejectedBatchRestoresTargets:") for assertion in assertions):
            raise RuntimeError("Go replay setup, outcome, or hook failed; inspect " + str(log))
        failures += 1
    if status == 0 and failures == 0:
        verdict = "pass"
    elif status == 1 and failures:
        verdict = "violation"
    else:
        raise RuntimeError("Go replay did not finish checking the invariant; inspect " + str(log))
    print(name + ": " + verdict + "; failing cases=" + str(failures) + " (" + str(log) + ")", flush=True)
    return verdict, failures


def self_test(out, env, java, jar):
    # Isolate a backup fault from the already-known stale-precondition finding.
    if model(out, env, java, jar, "control-model", allow_stale=False) != "pass":
        raise RuntimeError("unmodified backup-failure model must pass the control")
    if replay(out, env, "control-replay", CONTROL)[0] != "pass":
        raise RuntimeError("unmodified backup-failure code must pass the control")
    original = SOURCE.read_text()
    anchor = "if err := renameTreePath(item.change.Target, item.backup); err != nil {\n\t\t\t\tif rollbackErr := rollbackFilesystemChanges(items[:committed]);"
    if original.count(anchor) != 1:
        raise RuntimeError("backup-error branch changed; review the mutation control")
    mutant = original.replace(anchor, anchor.replace("items[:committed]", "items[:0]"))
    scratch = out / "mutant.go"
    scratch.write_text(mutant)
    overlay = out / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(scratch)}}))
    if model(out, env, java, jar, "mutant-model", allow_stale=False, rollback=False) != "violation":
        raise RuntimeError("model missed the removed backup-error rollback")
    if replay(out, env, "mutant-replay", CONTROL, overlay)[0] != "violation":
        raise RuntimeError("real-code replay missed the removed backup-error rollback")
    if SOURCE.read_text() != original:
        raise RuntimeError("product source changed during the overlay-only mutation check")
    if replay(out, env, "restored-control-replay", CONTROL)[0] != "pass":
        raise RuntimeError("unmodified control did not pass after dropping the overlay")
    print("Mutation detected in both model and real code; original source unchanged.", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, help="new evidence directory outside the repository")
    parser.add_argument("--self-test", action="store_true", help="also check a scratch rollback-removal mutation")
    args = parser.parse_args()
    if args.out:
        out = args.out.resolve()
        if out == ROOT or ROOT in out.parents:
            parser.error("--out must be outside the repository")
        out.mkdir(parents=True, exist_ok=False)
    else:
        out = Path(tempfile.mkdtemp(prefix="packy-projection-proof-"))
    print("Evidence: " + str(out), flush=True)
    java, jar = tools()
    env = environment(out)
    print("TLC 1.7.4; Temurin 21.0.13+9; two targets, 320 replay inputs", flush=True)
    if args.self_test:
        self_test(out, env, java, jar)
    model_verdict = model(out, env, java, jar, "model")
    replay_verdict, failures = replay(out, env, "replay")
    summary = {"model": model_verdict, "replay": replay_verdict, "failing_replay_cases": failures,
               "java": str(java), "tla2tools": str(jar), "self_test": args.self_test}
    (out / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print("Read every model result with $statechart; copied inputs and view.mjs are in the evidence directories.", flush=True)
    return int(model_verdict != "pass" or replay_verdict != "pass")


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print("Verification infrastructure error: " + str(error), file=sys.stderr)
        sys.exit(2)
