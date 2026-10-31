#!/usr/bin/env python3
"""Observe a release run and independently check a pending approval."""
import argparse
from dataclasses import dataclass
import json
import re
import signal
import subprocess
import sys
import time

REPO = "yersonargotev/packy"
BASE = f"repos/{REPO}"
STATUSES = {"queued", "in_progress", "completed", "waiting", "requested", "pending"}
CONCLUSIONS = {"success", "failure", "cancelled", "timed_out", "action_required", "neutral", "skipped", "stale", "startup_failure"}


class Refusal(Exception):
    pass


@dataclass(frozen=True)
class Target:
    run: int
    tag: str
    sha: str


@dataclass(frozen=True)
class Job:
    id: int
    status: str
    conclusion: str
    steps: tuple


@dataclass(frozen=True)
class Snapshot:
    attempt: int
    status: str
    conclusion: str
    jobs: tuple


def require(condition, message):
    if not condition:
        raise Refusal(message)


def integer(value):
    require(type(value) is int and value > 0, "invalid numeric field")
    return value


def state(value, allowed):
    require(isinstance(value, str) and value in allowed, "invalid state")
    return value


def conclusion(value):
    if value is None or value == "":
        return ""
    return state(value, CONCLUSIONS)


def gh(*args):
    try:
        result = subprocess.run(["gh", *args], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=False)
        require(result.returncode == 0, f"GitHub read failed (exit {result.returncode}); restore API/auth access")
        return json.loads(result.stdout)
    except (OSError, ValueError):
        raise Refusal("GitHub read unavailable or malformed; restore API/auth access") from None


def snapshot(target):
    data = gh("run", "view", str(target.run), "--repo", REPO, "--json",
              "databaseId,attempt,event,headBranch,headSha,status,conclusion,jobs")
    require(isinstance(data, dict), "invalid run response")
    require(integer(data.get("databaseId")) == target.run and integer(data.get("attempt")) == 1
            and data.get("event") == "push" and data.get("headBranch") == target.tag
            and data.get("headSha") == target.sha, "run identity mismatch")
    status = state(data.get("status"), STATUSES)
    result = conclusion(data.get("conclusion"))
    require((status == "completed") == bool(result), "inconsistent run state")
    raw_jobs = data.get("jobs")
    require(isinstance(raw_jobs, list), "invalid jobs")
    jobs = []
    for job in raw_jobs:
        require(isinstance(job, dict), "invalid job")
        job_id = integer(job.get("databaseId"))
        job_status = state(job.get("status"), STATUSES)
        job_conclusion = conclusion(job.get("conclusion"))
        require((job_status == "completed") == bool(job_conclusion), "inconsistent job state")
        raw_steps = job.get("steps")
        require(isinstance(raw_steps, list), "invalid steps")
        steps = []
        for step in raw_steps:
            require(isinstance(step, dict), "invalid step")
            number = integer(step.get("number"))
            step_status = state(step.get("status"), STATUSES)
            step_conclusion = conclusion(step.get("conclusion"))
            require((step_status == "completed") == bool(step_conclusion), "inconsistent step state")
            steps.append((number, step_status, step_conclusion))
        require(len({s[0] for s in steps}) == len(steps), "duplicate step")
        jobs.append(Job(job_id, job_status, job_conclusion, tuple(sorted(steps))))
    require(len({j.id for j in jobs}) == len(jobs), "duplicate job")
    return Snapshot(1, status, result, tuple(sorted(jobs, key=lambda j: j.id)))


def pending(target):
    data = gh("api", f"{BASE}/actions/runs/{target.run}/pending_deployments")
    require(isinstance(data, list), "invalid pending deployments")
    gates = []
    for entry in data:
        require(isinstance(entry, dict) and isinstance(entry.get("environment"), dict), "invalid pending deployment")
        env = entry["environment"]
        name = env.get("name")
        env_id = integer(env.get("id"))
        eligible = entry.get("current_user_can_approve")
        require(isinstance(name, str) and type(eligible) is bool, "invalid pending deployment")
        # Unknown names are never printed and cannot authorize either known gate.
        if name in ("release", "homebrew"):
            gates.append((name, env_id, eligible))
    return tuple(sorted(gates))


def waiting(snap):
    return snap.status == "waiting" or any(job.status == "waiting" for job in snap.jobs)


def watch(target):
    previous = None
    last_gates = None
    while True:
        current = snapshot(target)
        if previous != current:
            print(f"run {target.run} attempt {current.attempt} {current.status} {current.conclusion}".rstrip(), flush=True)
            old = {job.id: job for job in previous.jobs} if previous else {}
            for job in current.jobs:
                if old.get(job.id) != job:
                    active = ",".join(str(n) for n, s, _ in job.steps if s in ("in_progress", "waiting")) or "none"
                    print(f"job {job.id} {job.status} {job.conclusion or '-'} active steps {active} https://github.com/{REPO}/actions/runs/{target.run}/job/{job.id}", flush=True)
                    if job.conclusion and job.conclusion not in ("success", "skipped", "neutral"):
                        failed = ",".join(str(n) for n, _, c in job.steps if c and c not in ("success", "skipped", "neutral")) or "unknown"
                        print(f"Stop publication. Failed steps {failed}. Diagnose with gh run view --repo {REPO} --job {job.id} --log-failed; filter sensitive logs before retention.", flush=True)
        else:
            print(f"run {target.run} heartbeat {current.status}", flush=True)
        if waiting(current):
            gates = pending(target)
            if gates != last_gates:
                if not gates:
                    print("waiting; no recognized pending environment", flush=True)
                for name, env_id, eligible in gates:
                    print(f"waiting {name} environment {env_id} eligible {str(eligible).lower()}", flush=True)
            last_gates = gates
        else:
            last_gates = None
        if current.status == "completed":
            if current.conclusion == "success":
                return 0
            raise Refusal("run did not succeed; stop publication")
        previous = current
        time.sleep(45)


def check_approval(target, name, env_id):
    data = gh("api", f"{BASE}/actions/runs/{target.run}")
    require(isinstance(data, dict), "invalid run response")
    repo = data.get("repository")
    require(isinstance(repo, dict) and repo.get("full_name") == REPO
            and integer(data.get("id")) == target.run and integer(data.get("run_attempt")) == 1
            and data.get("event") == "push" and data.get("path") == ".github/workflows/release.yml"
            and data.get("head_branch") == target.tag and data.get("head_sha") == target.sha,
            "run identity mismatch")
    status = state(data.get("status"), STATUSES)
    require(status in ("waiting", "in_progress"), "run is not waiting")
    # A waiting job can coexist with an in-progress run. Read fresh jobs in that case.
    if status != "waiting":
        pages = gh("api", "--paginate", "--slurp", f"{BASE}/actions/runs/{target.run}/attempts/1/jobs")
        require(isinstance(pages, list) and pages, "jobs unavailable")
        statuses = []
        for page in pages:
            require(isinstance(page, dict) and isinstance(page.get("jobs"), list), "jobs unavailable")
            for job in page["jobs"]:
                require(isinstance(job, dict), "invalid job")
                statuses.append(state(job.get("status"), STATUSES))
        require("waiting" in statuses, "run is not waiting")
    ref = gh("api", f"{BASE}/git/ref/tags/{target.tag}")
    require(isinstance(ref, dict) and ref.get("ref") == f"refs/tags/{target.tag}"
            and isinstance(ref.get("object"), dict), "tag identity unavailable")
    obj = ref["object"]
    if obj.get("type") == "tag":
        tag_sha = obj.get("sha")
        require(isinstance(tag_sha, str) and re.fullmatch(r"[0-9a-f]{40}", tag_sha), "invalid tag object")
        annotated = gh("api", f"{BASE}/git/tags/{tag_sha}")
        require(isinstance(annotated, dict) and annotated.get("tag") == target.tag
                and isinstance(annotated.get("object"), dict), "invalid tag object")
        obj = annotated["object"]
    require(obj.get("type") == "commit" and obj.get("sha") == target.sha, "tag commit mismatch")
    gates = pending(target)
    require(len([gate for gate in gates if gate[0] == name]) == 1
            and (name, env_id, True) in gates, "pending environment or eligibility mismatch")
    print(f"approval check passed: run {target.run} attempt 1 {name} environment {env_id}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="operation", required=True)
    for operation in ("watch", "check-approval"):
        command = sub.add_parser(operation)
        command.add_argument("--run", required=True, type=int)
        command.add_argument("--tag", required=True)
        command.add_argument("--sha", required=True)
        if operation == "check-approval":
            command.add_argument("--environment", choices=("release", "homebrew"), required=True)
            command.add_argument("--environment-id", type=int, required=True)
    args = parser.parse_args()
    try:
        integer(args.run)
        require(re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", args.tag), "invalid tag")
        require(re.fullmatch(r"[0-9a-f]{40}", args.sha), "invalid SHA")
        target = Target(args.run, args.tag, args.sha)
        if args.operation == "watch":
            return watch(target)
        integer(args.environment_id)
        check_approval(target, args.environment, args.environment_id)
        return 0
    except Refusal as error:
        print(f"Refused: {error}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("Observation interrupted; completion is unknown.", file=sys.stderr)
        return 130


def interrupted(_signal, _frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, interrupted)
    sys.exit(main())
