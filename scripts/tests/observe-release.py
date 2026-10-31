#!/usr/bin/env python3
"""Exercise the observer against an inert gh executable."""
import json
import os
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SHA = "a" * 40
SECRET = "credential-SENTINEL-https://example.test/?signature=secret"
RUN = 123
TAG = "v1.2.3"
TARGET = ["--run", str(RUN), "--tag", TAG, "--sha", SHA]

FAKE = '''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
root = Path(os.environ["FAKE_ROOT"])
args = sys.argv[1:]
with (root / "calls").open("a") as log: log.write(json.dumps(args) + "\\n")
scenario = json.loads((root / "scenario").read_text())
if scenario.get("malformed") and scenario["malformed"] in " ".join(args):
    print("not-json")
    sys.exit(0)
if scenario.get("fail") and scenario["fail"] in " ".join(args):
    print("credential-SENTINEL-https://example.test/?signature=secret", file=sys.stderr)
    sys.exit(1)
if args[:2] == ["run", "watch"]:
    for item in scenario["snapshots"]:
        print("run", item["status"], item["conclusion"] or "-", "job", item["jobs"][0]["status"])
    sys.exit(0)
if args[:2] == ["run", "view"]:
    counter = root / "index"
    index = int(counter.read_text()) if counter.exists() else 0
    counter.write_text(str(index + 1))
    result = scenario["snapshots"][min(index, len(scenario["snapshots"]) - 1)]
elif "pending_deployments" in args[-1]:
    counter = root / "index"
    index = str(int(counter.read_text())) if counter.exists() else "0"
    result = scenario.get("pending_by_index", {}).get(index, scenario["pending"])
elif "/git/ref/" in args[-1]: result = scenario["ref"]
elif "/jobs" in args[-1]: result = scenario["jobs"]
elif "/actions/runs/" in args[-1]: result = scenario["run"]
else: sys.exit(2)
print(json.dumps(result))
'''


def snapshot(status="in_progress", job_status="in_progress", step_status="in_progress", result=""):
    return {"databaseId": RUN, "attempt": 1, "event": "push", "headBranch": TAG,
            "headSha": SHA, "status": status, "conclusion": result,
            "jobs": [{"databaseId": 456, "name": SECRET, "status": job_status,
                      "conclusion": result if job_status == "completed" else "",
                      "steps": [{"number": 1, "name": SECRET, "status": step_status,
                                 "conclusion": result if step_status == "completed" else ""}]}]}


def scenario():
    return {"snapshots": [snapshot()],
            "run": {"id": RUN, "run_attempt": 1, "event": "push", "path": ".github/workflows/release.yml",
                    "head_branch": TAG, "head_sha": SHA, "repository": {"full_name": "yersonargotev/packy"}, "status": "waiting"},
            "ref": {"ref": "refs/tags/" + TAG, "object": {"type": "commit", "sha": SHA}},
            "pending": [{"environment": {"name": "release", "id": 11}, "current_user_can_approve": True},
                        {"environment": {"name": "homebrew", "id": 22}, "current_user_can_approve": True}],
            "jobs": [{"jobs": [{"status": "waiting"}]}]}


class ObserverTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        (self.root / "gh").write_text(FAKE)
        (self.root / "gh").chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ["PATH"], FAKE_ROOT=str(self.root))

    def tearDown(self):
        self.temp.cleanup()

    def invoke(self, data, operation, extra=(), fast=False):
        (self.root / "scenario").write_text(json.dumps(data))
        for file in ("calls", "index"):
            (self.root / file).unlink(missing_ok=True)
        script = str(ROOT / "scripts/observe-release.py")
        if fast:
            code = "import runpy,sys; d=runpy.run_path(sys.argv.pop(1)); d['time'].sleep=lambda _:None; sys.exit(d['main']())"
            command = [sys.executable, "-B", "-c", code, script]
        else:
            command = [sys.executable, "-B", script]
        result = subprocess.run(command + [operation, *TARGET, *extra], env=self.env, capture_output=True, text=True, timeout=20)
        calls = [json.loads(line) for line in (self.root / "calls").read_text().splitlines()] if (self.root / "calls").exists() else []
        self.assertNotIn(SECRET, result.stdout + result.stderr)
        self.assertTrue(all(not any(word in call for word in ("POST", "PATCH", "DELETE", "PUT")) for call in calls))
        return result, calls

    def test_watch_transitions_waits_and_completion(self):
        data = scenario()
        data["snapshots"] = [snapshot(), snapshot(), snapshot(job_status="queued", step_status="queued"),
                             snapshot(job_status="waiting", step_status="queued"), snapshot(job_status="waiting", step_status="queued"),
                             snapshot(), snapshot(status="waiting", job_status="waiting", step_status="queued"),
                             snapshot(status="completed", job_status="completed", step_status="completed", result="success")]
        data["pending_by_index"] = {"4": [data["pending"][0]], "5": [data["pending"][0]], "7": [data["pending"][1]]}
        result, calls = self.invoke(data, "watch", fast=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sum(call[:2] == ["run", "view"] for call in calls), 8)
        self.assertEqual(sum("pending_deployments" in call[-1] for call in calls), 3)
        self.assertEqual(result.stdout.count("waiting release environment"), 1)
        self.assertEqual(result.stdout.count("waiting homebrew environment"), 1)
        self.assertEqual(result.stdout.count("heartbeat"), 2)
        self.assertIn("completed success", result.stdout)

    def test_failure_is_actionable_and_read_errors_are_safe(self):
        data = scenario()
        data["snapshots"] = [snapshot(status="completed", job_status="completed", step_status="completed", result="failure")]
        result, _ = self.invoke(data, "watch", fast=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("--job 456 --log-failed", result.stdout)
        self.assertIn("Failed steps 1", result.stdout)
        data["fail"] = "run view"
        result, _ = self.invoke(data, "watch", fast=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("GitHub read failed", result.stderr)

    def test_watch_refuses_malformed_or_changed_identity(self):
        for field, value in (("databaseId", True), ("attempt", True), ("attempt", 2),
                             ("event", "pull_request"), ("headBranch", "v2.0.0"),
                             ("headSha", "b" * 40), ("status", "unknown"), ("jobs", None)):
            data = scenario()
            data["snapshots"][0][field] = value
            result, _ = self.invoke(data, "watch", fast=True)
            self.assertNotEqual(result.returncode, 0, field)
        data = scenario()
        data["snapshots"][0]["jobs"][0]["steps"][0]["number"] = True
        result, _ = self.invoke(data, "watch", fast=True)
        self.assertNotEqual(result.returncode, 0)

    def test_interrupt_and_resume_the_same_run(self):
        data = scenario()
        (self.root / "scenario").write_text(json.dumps(data))
        command = [sys.executable, "-B", str(ROOT / "scripts/observe-release.py"), "watch", *TARGET]
        first = subprocess.Popen(command, env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            self.assertIn("run 123", first.stdout.readline())
            first.send_signal(signal.SIGTERM)
            first_output, first_error = first.communicate(timeout=5)
            self.assertNotEqual(first.returncode, 0)
            self.assertIn("interrupted", first_error)
            data["snapshots"] = [snapshot(status="completed", job_status="completed", step_status="completed", result="success")]
            (self.root / "scenario").write_text(json.dumps(data))
            second = subprocess.run(command, env=self.env, capture_output=True, text=True, timeout=5)
            self.assertEqual(second.returncode, 0, second.stderr)
            self.assertIn("completed success", second.stdout)
        finally:
            if first.poll() is None:
                first.kill()
                first.wait()

    def test_each_approval_requires_all_fresh_identity_fields(self):
        for name, env_id in (("release", 11), ("homebrew", 22)):
            extra = ["--environment", name, "--environment-id", str(env_id)]
            result, calls = self.invoke(scenario(), "check-approval", extra)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(len(calls), 3)
            mutations = [
                ("run", "id", 999), ("run", "id", True), ("run", "run_attempt", 2), ("run", "run_attempt", True), ("run", "event", "pull_request"),
                ("run", "path", "other"), ("run", "head_branch", "v9.9.9"), ("run", "head_sha", "b" * 40),
                ("run", "repository", {"full_name": "elsewhere/repo"}), ("run", "status", "completed"),
                ("ref", "ref", "refs/tags/v9.9.9"), ("ref", "object", {"type": "commit", "sha": "b" * 40}),
            ]
            for section, field, value in mutations:
                data = scenario(); data[section][field] = value
                result, _ = self.invoke(data, "check-approval", extra)
                self.assertNotEqual(result.returncode, 0, (name, field))
                data = scenario(); del data[section][field]
                result, _ = self.invoke(data, "check-approval", extra)
                self.assertNotEqual(result.returncode, 0, (name, field, "missing"))
            for replacement in ([], [{"environment": {"name": "other", "id": env_id}, "current_user_can_approve": True}],
                                [{"environment": {"name": name, "id": env_id}, "current_user_can_approve": "true"}],
                                [{"environment": {"name": name, "id": env_id + 1}, "current_user_can_approve": True}],
                                [{"environment": {"name": name, "id": env_id}, "current_user_can_approve": False}],
                                [{"environment": {"name": name, "id": env_id}}],
                                [{"environment": {"name": name, "id": env_id}, "current_user_can_approve": True}] * 2):
                data = scenario(); data["pending"] = replacement
                result, _ = self.invoke(data, "check-approval", extra)
                self.assertNotEqual(result.returncode, 0, (name, replacement))
            for failing in ("actions/runs/123", "git/ref/tags", "pending_deployments"):
                data = scenario(); data["fail"] = failing
                result, _ = self.invoke(data, "check-approval", extra)
                self.assertNotEqual(result.returncode, 0, (name, failing))
            for malformed in ("actions/runs/123", "git/ref/tags", "pending_deployments"):
                data = scenario(); data["malformed"] = malformed
                result, _ = self.invoke(data, "check-approval", extra)
                self.assertNotEqual(result.returncode, 0, (name, malformed))
            data = scenario(); data["run"]["status"] = "in_progress"
            result, calls = self.invoke(data, "check-approval", extra)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(len(calls), 4)
            data["jobs"] = [{"jobs": [{"status": "queued"}]}]
            result, _ = self.invoke(data, "check-approval", extra)
            self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
