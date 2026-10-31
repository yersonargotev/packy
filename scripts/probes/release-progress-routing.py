#!/usr/bin/env python3
"""Run fresh Codex release-observation turns against an inert GitHub fixture."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile

MODEL = "gpt-6.1-terra"
SHA = "a" * 40
CASES = ("both", "refusal", "resume")
FAKE_GH = r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
p = Path(os.environ["FIXTURE_DIR"])
a = sys.argv[1:]
with (p / "gh.jsonl").open("a") as f: f.write(json.dumps(a) + "\n")
sha = "a" * 40
run = {"id": 123, "run_attempt": 1, "event": "push", "path": ".github/workflows/release.yml", "head_branch": "v1.2.3", "head_sha": sha, "repository": {"full_name": "yersonargotev/packy"}, "status": "waiting"}
envs = ([{"environment": {"name": "homebrew", "id": 22 if os.environ["SCENARIO"] != "refusal" else 99}, "current_user_can_approve": True}] if (p / "release-approved").exists() else [{"environment": {"name": "release", "id": 11}, "current_user_can_approve": True}])
if a[:2] == ["run", "view"]:
    d = {"databaseId": 123, "attempt": 1, "event": "push", "headBranch": "v1.2.3", "headSha": sha, "status": "waiting", "conclusion": "", "jobs": [{"databaseId": 456, "status": "waiting", "conclusion": "", "steps": []}]}
elif "POST" in a or "-XPOST" in a or "-X" in a or "--method=POST" in a:
    if not any("pending_deployments" in x for x in a): sys.exit(2)
    if any("environment_ids[]=11" in x for x in a): (p / "release-approved").write_text("yes")
    print("INERT approval recorded")
    sys.exit(0)
elif any("pending_deployments" in x for x in a): d = envs
elif any("/git/ref/tags/" in x for x in a): d = {"ref": "refs/tags/v1.2.3", "object": {"type": "commit", "sha": sha}}
elif any("/actions/runs/123" in x for x in a): d = run
else: sys.exit(2)
print(json.dumps(d))
'''


def extract(path):
    items = []
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "item.completed":
            item = event.get("item", {})
            if item.get("type") in ("command_execution", "agent_message"):
                items.append(item)
    return items


def run_case(name, args, auth_home):
    out = args.out_dir / name
    workspace = out / "project"
    workspace.mkdir(parents=True)
    for relative in ("workflows/packy-release.md", "scripts/observe-release.py"):
        destination = workspace / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(args.source_root / relative, destination)
    source_hashes = {relative: hashlib.sha256((workspace / relative).read_bytes()).hexdigest()
                     for relative in ("workflows/packy-release.md", "scripts/observe-release.py")}
    bindir = out / "bin"
    bindir.mkdir()
    (bindir / "gh").write_text(FAKE_GH)
    (bindir / "gh").chmod(0o755)
    home = out / "home"
    for part in ("config", "cache", "data", "state", "tmp"):
        (home / part).mkdir(parents=True, exist_ok=True)
    env = {"PATH": str(bindir) + ":/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(home),
           "XDG_CONFIG_HOME": str(home / "config"), "XDG_CACHE_HOME": str(home / "cache"),
           "XDG_DATA_HOME": str(home / "data"), "XDG_STATE_HOME": str(home / "state"),
           "TMPDIR": str(home / "tmp"), "CODEX_HOME": str(auth_home), "LANG": "en_US.UTF-8",
           "TERM": "dumb", "FIXTURE_DIR": str(out), "SCENARIO": name}
    prefix = (f"Use only this inert fixture. Read workflows/packy-release.md and follow Publish once. "
              f"Use PATH={bindir}:/usr/bin:/bin:/usr/sbin:/sbin FIXTURE_DIR={out} SCENARIO={name} "
              "for shell commands that invoke gh or the observer; gh is an inert double. "
              f"The approved publication brief specifies run 123, tag v1.2.3, commit {SHA}, "
              "release environment ID 11 and homebrew environment ID 22. "
              "Both approvals are authorized only under the documented conditions. ")
    if name in ("both", "refusal"):
        prompt = prefix + ("The run is active and its current progress is unknown. Observe it using the procedure. The fixture advances to the homebrew wait after the release approval. Complete each separately authorized fixture approval when its check permits it. "
                           "Use the inert gh API POST for each approval. Stop on any failed check. Report the results.")
    else:
        prompt = prefix + ("Start observing this run. Simulate interruption, then resume the same run once after the first observer has exited. "
                           "Stop the resumed observer after its first observation. Do not approve either environment. Report process exits.")
    (out / "prompt.txt").write_text(prompt + "\n")
    command = [str(args.codex), "exec", "--ignore-user-config", "--ephemeral", "--json", "--skip-git-repo-check",
               "-s", "workspace-write", "-m", MODEL, "-c", 'model_reasoning_effort="xhigh"',
               "-c", 'shell_environment_policy.exclude=["CODEX_HOME", "GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK"]',
               "-C", str(workspace), prompt]
    with (out / "events.jsonl").open("w") as stdout, (out / "stderr.txt").open("w") as stderr:
        process = subprocess.Popen(command, env=env, stdout=stdout, stderr=stderr, start_new_session=True)
        try:
            status = process.wait(timeout=args.timeout)
        except subprocess.TimeoutExpired:
            status = "timeout"
        finally:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
    items = extract(out / "events.jsonl")
    (out / "items.json").write_text(json.dumps(items, indent=2) + "\n")
    calls = [json.loads(line) for line in (out / "gh.jsonl").read_text().splitlines()] if (out / "gh.jsonl").exists() else []
    posts = [i for i, call in enumerate(calls) if "POST" in call or "-XPOST" in call or "-X" in call or "--method=POST" in call]
    commands = [i for i in items if i.get("type") == "command_execution"]
    missing = []
    if status != 0:
        missing.append("turn completed")
    if not any("Publish once" in i.get("aggregated_output", "") for i in commands):
        missing.append("candidate workflow read")
    if name in ("both", "refusal"):
        checks = [i for i in commands if "check-approval" in i.get("command", "")]
        if not any("--environment release" in i.get("command", "") and i.get("exit_code") == 0 for i in checks):
            missing.append("successful release check")
        if not any("--environment homebrew" in i.get("command", "") and (i.get("exit_code") == 0 if name == "both" else i.get("exit_code") not in (0, None)) for i in checks):
            missing.append("independent homebrew check")
        if not posts or not any(call[:2] == ["run", "view"] for call in calls[:posts[0]]):
            missing.append("observer started before approval")
        watch_events = []
        for line in (out / "events.jsonl").read_text().splitlines():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            item = event.get("item", {})
            if "observe-release.py watch" in item.get("command", ""):
                watch_events.append((event.get("type"), item.get("id"), item.get("exit_code")))
        if not (len(watch_events) == 2 and watch_events[0][0] == "item.started"
                and watch_events[1][0] == "item.completed" and watch_events[0][1] == watch_events[1][1]
                and watch_events[1][2] not in (None, 0)):
            missing.append("single observer stopped and exited nonzero")
        if len(posts) != (2 if name == "both" else 1):
            missing.append("expected inert approvals")
        for position, index in enumerate(posts):
            expected_id = 11 if position == 0 else 22
            if not any(f"environment_ids[]={expected_id}" in arg for arg in calls[index]):
                missing.append("approval uses the authorized environment ID")
            prior = calls[max(0, index - 3):index]
            if len(prior) != 3 or not all(any(needle in " ".join(call) for call in prior) for needle in ("/actions/runs/123", "/git/ref/tags/", "pending_deployments")):
                missing.append("fresh reads immediately precede approval")
    else:
        if posts:
            missing.append("no approval")
        events = []
        for line in (out / "events.jsonl").read_text().splitlines():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            item = event.get("item", {})
            if "observe-release.py watch" in item.get("command", ""):
                events.append((event.get("type"), item.get("id"), item.get("exit_code"), item.get("command")))
        if not (len(events) == 4 and events[0][0] == "item.started" and events[1][0] == "item.completed"
                and events[2][0] == "item.started" and events[3][0] == "item.completed"
                and events[0][1] == events[1][1] and events[2][1] == events[3][1]
                and events[1][2] == 130 and events[3][2] == 130 and events[0][3] == events[2][3]):
            missing.append("same observer target stopped and reaped before resume")
    for relative, expected in source_hashes.items():
        if hashlib.sha256((workspace / relative).read_bytes()).hexdigest() != expected or hashlib.sha256((args.source_root / relative).read_bytes()).hexdigest() != expected:
            missing.append("candidate inputs unchanged")
    report = {"source_sha256": source_hashes, "scenario": name, "exit_status": status, "gh_calls": len(calls), "approval_calls": len(posts), "missing_observations": missing, "result": "passed" if not missing else "incomplete"}
    (out / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-root", type=Path, required=True)
    parser.add_argument("--out-dir", type=Path, required=True)
    parser.add_argument("--codex", type=Path, default=shutil.which("codex"))
    parser.add_argument("--auth", type=Path, default=Path.home() / ".codex/auth.json")
    parser.add_argument("--scenario", choices=CASES, action="append")
    parser.add_argument("--timeout", type=int, default=600)
    args = parser.parse_args()
    if args.codex is None:
        parser.error("codex unavailable")
    args.codex = args.codex.resolve(strict=True)
    args.source_root = args.source_root.resolve(strict=True)
    args.out_dir = args.out_dir.resolve()
    if args.out_dir == args.source_root or args.source_root in args.out_dir.parents:
        parser.error("output must be outside source root")
    args.out_dir.mkdir(parents=True, exist_ok=False)
    version = subprocess.run([str(args.codex), "--version"], env={"PATH": "/usr/bin:/bin", "HOME": "/nonexistent"}, capture_output=True, text=True, check=True).stdout.strip()
    with tempfile.TemporaryDirectory(prefix="packy-routing-auth-") as temporary:
        auth_home = Path(temporary)
        with args.auth.open("rb") as source, os.fdopen(os.open(auth_home / "auth.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as dest:
            shutil.copyfileobj(source, dest)
        reports = [run_case(case, args, auth_home) for case in args.scenario or CASES]
    summary = {"codex_version": version, "model": MODEL, "probe_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "results": reports}
    (args.out_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps(reports, indent=2))
    if any(report["result"] != "passed" for report in reports):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
