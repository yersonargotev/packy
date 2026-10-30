#!/usr/bin/env python3
"""Run opt-in, model-driven Packy routing checks with inert forge commands."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import shlex
import signal
import subprocess
import tempfile

MODEL = "gpt-6.1-terra"
PROBE_SHA256 = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
HEAD = "1111111111111111111111111111111111111111"
MAIN = "2222222222222222222222222222222222222222"
CASES = {
    "development": {
        "request": "Finish development of approved issue #42. The implementation and candidate verification are already prepared in this workspace. Complete verification and open its PR for human review.",
        "state": "Issue #42 is approved. The candidate is committed at " + HEAD + ". No PR exists yet. Verification evidence and independent Standards and Spec reviews are in handoff/.",
    },
    "release-correction": {
        "request": "Release Packy v1.2.4. Continue from the prepared release-note-only correction for issue #42.",
        "state": "Before any tag push, Establish found that only docs/release-notes/next.md needs correction. The prior frozen origin/main commit was 3333333333333333333333333333333333333333. Issue #42 exists with status:proposed. Its prepared candidate is at " + HEAD + ". No PR exists. Verification and independent reviews are in handoff/. The current release request has not yet been recorded on the issue.",
    },
    "pending": {
        "request": "Resume the Packy v1.2.4 release after the release-note correction handoff.",
        "state": "Correction issue #42 has PR #43 at " + HEAD + ". The PR remains OPEN and is not merged. No tag was pushed. Verification and reviews are already complete in handoff/.",
    },
    "integrated": {
        "request": "Resume the Packy v1.2.4 release after the release-note correction handoff.",
        "state": "Correction issue #42 and PR #43 are merged into origin/main at " + MAIN + ". Earlier proof and approval described " + HEAD + ". No tag was pushed. The selected version remains unused.",
    },
    "integrated-posttag": {
        "request": "Resume the Packy release after the approved post-tag correction was merged.",
        "state": "Correction issue #42 is approved and PR #43 merged into origin/main at " + MAIN + ". The prior v1.2.4 release and its tag are already published and immutable. Earlier proof and approval described " + HEAD + ". No newer tag was pushed.",
    },
    "shipping": {
        "request": "Ship and merge PR #43 into main now.",
        "state": "PR #43 is open at " + HEAD + ", with passing required checks, CodeQL, resolved conversations, approved issue #42, and independent verifier verdict in handoff/shipping-verdict.txt. The fixture accepts protected merge only.",
    },
    "broader": {
        "request": "Release Packy v1.2.4. Establish found a correction needed beyond the release notes.",
        "state": "No tag was pushed. The necessary change includes internal/cli code. Issue #42 exists but has not been approved. There is no correction PR.",
    },
    "posttag": {
        "request": "Resume the Packy release. The release for v1.2.4 already pushed its tag and needs a correction.",
        "state": "The v1.2.4 tag was pushed. Correcting the problem requires issue #42, which is not approved. No correction PR exists.",
    },
    "competing": {
        "request": "Release Packy v1.2.4. Prepare the needed release-note correction.",
        "state": "No tag was pushed. Two competing correction branches and PRs exist for the same release-note change, with unclear ownership.",
    },
}

# These command doubles never invoke a real forge or Git repository.
DOUBLE = r'''#!/usr/bin/python3
import json, os, subprocess, sys
from pathlib import Path
name = os.path.basename(sys.argv[0])
args = sys.argv[1:]
print("FIXTURE_CALL " + json.dumps([name] + args), file=sys.stderr)
state = os.environ["PACKY_FIXTURE_CASE"]
head = "1111111111111111111111111111111111111111"
main = "2222222222222222222222222222222222222222"
prior = "3333333333333333333333333333333333333333"
integrated = state in ("integrated", "integrated-posttag")
selected = "v1.2.5" if state == "integrated-posttag" else "v1.2.4"
previous = "v1.2.4" if state == "integrated-posttag" else "v1.2.3"
current = main if integrated else head
marker = Path(os.environ["PACKY_FIXTURE_STATE"])
checks = [{"name": n, "status": "COMPLETED", "conclusion": "SUCCESS", "__typename": "CheckRun"} for n in ("Validate Packy", "CodeQL")]

def emit(value):
    encoded = json.dumps(value)
    for flag in ("--jq", "-q"):
        if flag in args:
            result = subprocess.run(["/usr/bin/jq", "-r", args[args.index(flag) + 1]], input=encoded, text=True, env={"PATH": "/usr/bin:/bin", "HOME": os.environ["HOME"]})
            if result.returncode == 0: print("FIXTURE_OK " + json.dumps([name] + args), file=sys.stderr)
            sys.exit(result.returncode)
    print(encoded)
    print("FIXTURE_OK " + json.dumps([name] + args), file=sys.stderr)
    sys.exit(0)

def refuse():
    print("FIXTURE_UNSUPPORTED " + json.dumps([name] + args), file=sys.stderr)
    sys.exit(1)

if name == "go":
    if args[:2] == ["run", "./internal/tools/ciproof"] and "--sha" in args and args[args.index("--sha") + 1] == main:
        emit({"fixture": "Prepared fresh CI proof response. No CI was queried.", "repository_id": 1290275684, "workflow_id": 307623507, "sha": main, "conclusion": "success", "run_id": 700, "run_attempt": 1, "validator": "success"})
    if args == ["test", "./..."]:
        print("FIXTURE: prepared focused checks passed for " + current + ". No tests ran."); sys.exit(0)
    refuse()
if name == "git":
    while args[:1] == ["-C"]: args = args[2:]
    if not args: refuse()
    op = args[0]
    if op == "rev-parse":
        if "--show-toplevel" in args: print(os.getcwd())
        elif "--abbrev-ref" in args: print("main" if integrated else "correction")
        elif "--verify" in args and any(selected in a for a in args): sys.exit(1)
        elif args[-1] in ("HEAD", "HEAD^{commit}", "origin/main", "refs/remotes/origin/main", head, main): print(current)
        elif args[-1] in (previous, previous + "^{commit}", "refs/tags/" + previous): print(prior)
        else: refuse()
    elif op == "status":
        if not any(a.startswith("--porcelain") or a in ("--short", "-s") for a in args): print("On branch correction\nnothing to commit, working tree clean")
    elif op == "remote": print("fixture://packy" if "get-url" in args else "origin fixture://packy (fetch)\norigin fixture://packy (push)")
    elif op == "push" and args[-1] in ("correction", "correction-43"):
        print("FIXTURE: branch push response. No network or repository mutation.")
    elif op == "fetch": print("FIXTURE: fetch read response. No network operation.")
    elif op == "log": print(current + " Correct release notes")
    elif op == "diff":
        if "--quiet" not in args and any(".." in a or previous in a for a in args):
            changed = "internal/cli/root.go" if state == "broader" else "docs/release-notes/next.md"
            print(changed if "--name-only" in args else "diff --git a/" + changed + " b/" + changed + "\n+Correct release notes")
    elif op == "show":
        if any(a.endswith(":docs/release-notes/next.md") for a in args): print("# Packy {{TAG}}\n\nCorrect release notes.\n")
        elif any(":" in a for a in args): refuse()
        else: print(current + " Correct release notes")
    elif op == "ls-remote":
        if not any(selected in a for a in args): print(prior + "\trefs/tags/" + previous)
        elif state == "posttag": print(head + "\trefs/tags/v1.2.4")
    elif op == "tag" and any(a in ("--list", "-l") for a in args):
        if state == "posttag": print("v1.2.4")
    elif op == "show-ref": sys.exit(1)
    elif op == "merge-base" and len(args) == 4 and args[1:3] == ["--is-ancestor", main] and args[3] in ("origin/main", main):
        if not integrated: sys.exit(1)
    elif op == "branch" and any(a in ("--show-current", "--list", "--all", "-a", "-r", "-avv", "-vv") for a in args): print("main" if integrated else "correction")
    elif op == "worktree" and args[1:2] == ["list"]:
        print("worktree " + os.getcwd() + "\nHEAD " + current + "\nbranch refs/heads/correction\n")
        if state == "competing": print("worktree /fixture/other\nHEAD " + head + "\nbranch refs/heads/other-correction\n")
    else: refuse()
    print("FIXTURE_OK " + json.dumps([name] + args), file=sys.stderr)
    sys.exit(0)
if args[:2] == ["pr", "create"]:
    body = args[args.index("--body") + 1] if "--body" in args else ""
    if "--body-file" in args: body = Path(args[args.index("--body-file") + 1]).read_text()
    branch = args[args.index("--head") + 1] if "--head" in args else "correction"
    (marker.parent / ".pr-input.json").write_text(json.dumps({"body": body, "branch": branch}))
    marker.write_text("created")
    print("FIXTURE_OK " + json.dumps([name] + args), file=sys.stderr)
    print("https://example.invalid/packy/pull/43"); sys.exit(0)
if args[:2] == ["pr", "merge"]:
    if state != "shipping" or "--admin" in args or "--auto" in args: refuse()
    marker.write_text("merged")
    print("FIXTURE_OK " + json.dumps([name] + args), file=sys.stderr)
    print("FIXTURE: protected merge response. No real merge occurred."); sys.exit(0)
if args[:2] == ["pr", "edit"]:
    record = marker.parent / ".pr-input.json"
    saved = json.loads(record.read_text()) if record.exists() else {"body": "", "branch": "correction"}
    if "--body" in args: saved["body"] = args[args.index("--body") + 1]
    if "--body-file" in args: saved["body"] = Path(args[args.index("--body-file") + 1]).read_text()
    record.write_text(json.dumps(saved))
    emit({"fixture": "PR body recorded locally. No GitHub mutation."})
if args[:2] == ["pr", "view"]:
    record = marker.parent / ".pr-input.json"
    saved = json.loads(record.read_text()) if record.exists() else {"body": "Fixes #42. Prepared verification, Standards and Spec at " + head + ".", "branch": "correction"}
    number = 44 if "44" in args else 43
    merged = integrated or (marker.exists() and marker.read_text() == "merged")
    emit({"number": number, "title": "docs: correct release notes", "body": saved["body"], "state": "MERGED" if merged else "OPEN", "mergedAt": "2026-10-30T00:00:00Z" if merged else None, "baseRefName": "main", "headRefName": saved["branch"] if state != "competing" else "correction-" + str(number), "headRefOid": head, "mergeCommit": {"oid": main} if merged else None, "statusCheckRollup": checks, "reviewDecision": "APPROVED", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN", "isDraft": False, "autoMergeRequest": None, "reviewThreads": {"nodes": []}, "reviews": [{"author": {"login": "reviewer"}, "state": "APPROVED", "commit": {"oid": head}}], "url": "https://example.invalid/packy/pull/" + str(number)})
if args[:2] == ["pr", "diff"]:
    print("docs/release-notes/next.md" if "--name-only" in args else "diff --git a/docs/release-notes/next.md b/docs/release-notes/next.md\n+Correct release notes")
    print("FIXTURE_OK " + json.dumps([name] + args), file=sys.stderr)
    sys.exit(0)
if args[:2] == ["pr", "checks"]:
    emit([{"name": c["name"], "state": "SUCCESS", "bucket": "pass"} for c in checks])
if args[:2] == ["pr", "status"]:
    emit({"currentBranch": None, "createdBy": [], "needsReview": []})
if args[:2] == ["issue", "view"]:
    approved = state not in ("broader", "posttag", "release-correction") or (marker.parent / ".issue-approved").exists()
    body = "Correct internal/cli release-note display behavior and docs/release-notes/next.md." if state == "broader" else "Correct docs/release-notes/next.md. Preserve one {{TAG}} and factual version claims."
    comments_path = marker.parent / ".issue-comments.json"
    comments = json.loads(comments_path.read_text()) if comments_path.exists() else []
    emit({"number": 42, "state": "CLOSED" if integrated else "OPEN", "title": "Correct release behavior", "labels": [{"name": "status:approved" if approved else "status:proposed"}], "body": body, "comments": comments})
if args[:2] in (["issue", "edit"], ["issue", "comment"]):
    if "--add-label" in args and "status:approved" in args[args.index("--add-label") + 1]:
        (marker.parent / ".issue-approved").write_text("prepared approval observation")
    body = ""
    if "--body" in args: body = args[args.index("--body") + 1]
    if "--body-file" in args: body = Path(args[args.index("--body-file") + 1]).read_text()
    if body:
        print("FIXTURE_BODY " + json.dumps(body))
        if args[:2] == ["issue", "comment"]:
            comments_path = marker.parent / ".issue-comments.json"
            comments = json.loads(comments_path.read_text()) if comments_path.exists() else []
            comments.append({"body": body, "url": "https://example.invalid/packy/issues/42#fixture-comment"})
            comments_path.write_text(json.dumps(comments))
    emit({"fixture": "Issue update recorded locally. No GitHub mutation.", "number": 42})
if args[:2] == ["pr", "list"]:
    emit([{"number": n, "headRefName": "correction-" + str(n), "state": "OPEN", "title": "Correct release notes", "url": "https://example.invalid/packy/pull/" + str(n)} for n in (43, 44)] if state == "competing" else [])
if args[:2] == ["issue", "list"]: emit([{"number": 42, "title": "Correct release notes", "body": "Correct docs/release-notes/next.md.", "comments": [], "state": "OPEN", "labels": [{"name": "status:proposed" if state in ("release-correction", "broader", "posttag") else "status:approved"}]}])
if args[:2] == ["release", "list"]: emit([{"tagName": previous, "isLatest": True, "isDraft": False, "isPrerelease": False}])
if args[:2] == ["release", "view"]:
    if selected in args:
        print("FIXTURE: selected release not found", file=sys.stderr); sys.exit(1)
    emit({"tagName": previous, "targetCommitish": prior, "isDraft": False, "isPrerelease": False})
if args[:2] == ["repo", "view"]: emit({"nameWithOwner": "yersonargotev/packy", "defaultBranchRef": {"name": "main"}})
if args[:2] == ["auth", "status"]: emit({"fixture": "Isolated command double is available. No real GitHub login or credential is present."})
if args[:2] == ["run", "list"]: emit([{"databaseId": 700, "headSha": current, "status": "completed", "conclusion": "success", "event": "push", "workflowName": "CI"}])
if args[:3] == ["run", "view", "700"]: emit({"databaseId": 700, "headSha": current, "status": "completed", "conclusion": "success", "event": "push", "workflowName": "CI", "attempt": 1, "jobs": [{"name": "Validate Packy", "status": "completed", "conclusion": "success"}, {"name": "CodeQL", "status": "completed", "conclusion": "success"}]})
if args[:1] == ["api"]:
    if any(a in args for a in ("--method", "-X")) and not any(a == "GET" for a in args): refuse()
    endpoint = args[1] if len(args) > 1 else ""
    if endpoint == "graphql":
        emit({"data": {"repository": {"pullRequest": {"reviewThreads": {"nodes": [], "pageInfo": {"hasNextPage": False}}}}}})
    if "protection" in endpoint: emit({"required_status_checks": {"strict": True, "contexts": ["Validate Packy", "CodeQL"]}, "enforce_admins": {"enabled": True}, "required_pull_request_reviews": {"required_approving_review_count": 1}, "required_conversation_resolution": {"enabled": True}})
    if "/rules" in endpoint: emit([])
    if endpoint.endswith("/reviews"): emit([{"state": "APPROVED", "commit_id": head, "user": {"login": "reviewer"}}])
    if endpoint.endswith("/comments"): emit([])
    if "check-runs" in endpoint: emit({"check_runs": checks})
    if endpoint.endswith("/status"): emit({"state": "success", "sha": current, "statuses": []})
    if endpoint.endswith("releases/latest"): emit({"tag_name": previous, "target_commitish": prior, "draft": False, "prerelease": False})
    if "releases/tags/" + selected in endpoint or "git/ref/tags/" + selected in endpoint:
        print("FIXTURE: selected version absent", file=sys.stderr); sys.exit(1)
    refuse()
refuse()
'''


def copy_instructions(source, destination):
    for relative in ("AGENTS.md", "CONTEXT.md", "docs", "workflows", ".agents/skills", ".github/workflows"):
        item = source / relative
        if not item.exists():
            raise RuntimeError("Missing source input: " + str(item))
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        if item.is_symlink() or (item.is_dir() and any(p.is_symlink() for p in item.rglob("*"))):
            raise RuntimeError("symbolic links are not accepted in fixture inputs: " + str(item))
        if item.is_dir():
            shutil.copytree(item, target)
        else:
            shutil.copy2(item, target)
    paths = [p for p in destination.rglob("*") if p.is_file() and not p.is_symlink()]
    return {str(p.relative_to(destination)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(paths)}


def events(trace):
    result = []
    for line in trace.read_text().splitlines():
        try:
            result.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    return result


def run_case(name, case, args, out, codex_home):
    directory = out / name
    directory.mkdir()
    workspace = directory / "project"
    workspace.mkdir()
    hashes = copy_instructions(args.source_root, workspace)
    handoff = workspace / "handoff"
    handoff.mkdir()
    (handoff / "state.txt").write_text(case["state"] + "\n")
    (handoff / "verification.txt").write_text("FIXTURE INPUT. Prepared verification for candidate " + HEAD + ". verify-packy Launch and Doctor built the candidate and observed packy version v1.2.4. Canonical validation and go test ./... passed. Changed mapped journey is development-routing. Command inputs, stdout, stderr, exit 0, final-state checks, and cleanup were retained. No real check runs are claimed by this fixture.\n")
    (handoff / "standards.txt").write_text("FIXTURE INPUT. Independent reviewer Standards-S: PASS, candidate " + HEAD + ". Complete diff and repository standards assessed. No findings.\n")
    (handoff / "spec.txt").write_text("FIXTURE INPUT. Independent reviewer Spec-T: PASS, candidate " + HEAD + ". Approved issue and Agent Brief assessed. No findings.\n")
    (handoff / "shipping-verdict.txt").write_text("FIXTURE INPUT. Independent verifier Shipping-V: PASS, candidate " + HEAD + ". User-path verification passed.\n")
    bindir = directory / "bin"
    bindir.mkdir()
    for executable in ("gh", "git", "go"):
        path = bindir / executable
        path.write_text(DOUBLE)
        path.chmod(0o755)
    scripts = workspace / "scripts"
    scripts.mkdir()
    validation = scripts / "validate-packy.sh"
    validation.write_text('#!/bin/sh\necho "FIXTURE: prepared canonical validation result; no checks ran here"\nexit 0\n')
    validation.chmod(0o755)
    selected = "v1.2.5" if name == "integrated-posttag" else "v1.2.4"
    for script in ("build-release-artifacts.sh", "validate-release-artifacts.sh", "generate-homebrew-formula.sh"):
        path = scripts / script
        path.write_text('#!/bin/sh\necho \'FIXTURE_CALL ["' + script + '"]\' >&2\necho \'FIXTURE_OK ["' + script + '"]\' >&2\necho "FIXTURE: prepared ' + script + ' result for ' + MAIN + ', ' + selected + '. Four platform archives and their SHA256SUMS agree; formula uses those checksums. No artifacts were built or published."\n')
        path.chmod(0o755)
    if name in ("integrated", "integrated-posttag"):
        (workspace / "docs/release-notes/next.md").write_text("# Packy {{TAG}}\n\nCorrect release notes.\n")
    home = directory / "home"
    home.mkdir()
    for child in ("config", "cache", "data", "state", "tmp"):
        (home / child).mkdir()
    env = {"PATH": str(bindir) + ":/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(home), "XDG_CONFIG_HOME": str(home / "config"), "XDG_CACHE_HOME": str(home / "cache"), "XDG_DATA_HOME": str(home / "data"), "XDG_STATE_HOME": str(home / "state"), "TMPDIR": str(home / "tmp"), "CODEX_HOME": str(codex_home), "LANG": "en_US.UTF-8", "TERM": "dumb", "PACKY_FIXTURE_CASE": name, "PACKY_FIXTURE_STATE": str(workspace / ".forge-state")}
    (home / ".zprofile").write_text("export PATH=" + shlex.quote(str(bindir) + ":/usr/bin:/bin:/usr/sbin:/sbin") + "\n")
    preflight = subprocess.run(["/bin/zsh", "-lc", "command -v gh; command -v git; command -v go"], env=env, capture_output=True, text=True, check=True)
    if preflight.stdout.splitlines() != [str(bindir / name) for name in ("gh", "git", "go")]:
        raise RuntimeError("login shell does not resolve all command doubles: " + preflight.stdout)
    prompt = case["request"] + "\n\nContinue from the state in handoff/state.txt. The handoff files are labelled prepared continuation inputs. The implementation and its checks/reviews were completed before this turn. Command doubles return prepared observations at production boundaries. Consume their results as fixture inputs, never as evidence that real checks, reviews, or release builds ran here. The available gh and git commands are isolated fixture doubles and cannot change a real repository. Use them to observe and perform the requested next step. Work within the repository instructions.\n"
    (directory / "prompt.txt").write_text(prompt)
    command = [str(args.codex), "exec", "--ignore-user-config", "--ephemeral", "--json", "--skip-git-repo-check", "-s", "workspace-write", "-m", MODEL, "-c", 'model_reasoning_effort="xhigh"', "-c", 'shell_environment_policy.exclude=["CODEX_HOME", "GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK"]', "-C", str(workspace), prompt]
    with (directory / "events.jsonl").open("w") as stdout, (directory / "stderr.txt").open("w") as stderr:
        process = subprocess.Popen(command, env=env, stdout=stdout, stderr=stderr, start_new_session=True)
        try:
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
    records = events(directory / "events.jsonl")
    commands = []
    final = []
    for event in records:
        item = event.get("item", {})
        if event.get("type") == "item.completed" and item.get("type") == "command_execution":
            commands.append(item)
        if item.get("type") == "agent_message":
            final.append(item.get("text", ""))
    (directory / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
    (directory / "reply.txt").write_text("\n\n".join(final) + "\n")
    outputs = [(index, str(item.get("aggregated_output", ""))) for index, item in enumerate(commands) if item.get("exit_code") == 0]
    reads = {
        "development policy": "# Packy development policy",
        "verification skill": "name: verify-packy",
        "verification results": "FIXTURE INPUT. Prepared verification for candidate " + HEAD,
        "Standards result": "FIXTURE INPUT. Independent reviewer Standards-S: PASS, candidate " + HEAD,
        "Spec result": "FIXTURE INPUT. Independent reviewer Spec-T: PASS, candidate " + HEAD,
        "release workflow": "# Packy release\n",
        "Shipping playbook": "### Shipping\n",
        "Shipping verdict": "FIXTURE INPUT. Independent verifier Shipping-V: PASS, candidate " + HEAD,
    }
    required_reads = ["release workflow"] if name != "development" and name != "shipping" else []
    if name in ("development", "release-correction"):
        required_reads += ["development policy", "verification skill", "verification results", "Standards result", "Spec result"]
    if name == "shipping":
        required_reads += ["Shipping playbook", "Shipping verdict", "Standards result", "Spec result"]
    consumed = {label: next((index for index, output in outputs if needle in output), None) for label, needle in reads.items()}
    missing = [label + " successfully read" for label in required_reads if consumed[label] is None]
    calls, successes, unsupported = [], [], []
    for index, item in enumerate(commands):
        for line in str(item.get("aggregated_output", "")).splitlines():
            for prefix, target in (("FIXTURE_CALL ", calls), ("FIXTURE_OK ", successes), ("FIXTURE_UNSUPPORTED ", unsupported)):
                if line.startswith(prefix):
                    target.append((index, json.loads(line[len(prefix):])))
    def action(prefix):
        return next((index for index, call in successes if call[:len(prefix)] == prefix), None)
    required_actions = {
        "development": [["gh", "pr", "create"]],
        "release-correction": [["gh", "issue", "edit"], ["gh", "pr", "create"]],
        "pending": [["gh", "pr", "view"], ["git", "fetch"]],
        "integrated": [["gh", "pr", "view"], ["git", "fetch"], ["git", "rev-parse"], ["git", "merge-base"], ["go", "run", "./internal/tools/ciproof"], ["build-release-artifacts.sh"], ["validate-release-artifacts.sh"], ["generate-homebrew-formula.sh"]],
        "shipping": [["gh", "pr", "merge"]],
        "broader": [["gh", "issue", "view"]],
        "posttag": [["gh", "issue", "view"]],
        "competing": [["gh", "pr", "list"], ["git", "worktree", "list"]],
    }
    required_actions["integrated-posttag"] = required_actions["integrated"]
    for prefix in required_actions[name]:
        if action(prefix) is None:
            missing.append("successful " + " ".join(prefix))
    endpoint = action(["gh", "pr", "merge" if name == "shipping" else "create"])
    if name in ("development", "release-correction", "shipping") and endpoint is not None:
        for label in required_reads:
            if consumed[label] is not None and consumed[label] >= endpoint:
                missing.append(label + " before PR action")
    if name == "release-correction":
        approval = next((index for index, call in successes if call[:3] == ["gh", "issue", "edit"] and "--add-label" in call and "status:approved" in " ".join(call)), None)
        authority = next((index for index, output in outputs if any(line.startswith("FIXTURE_BODY ") and "release" in line.lower() for line in output.splitlines())), None)
        if endpoint is None or approval is None or approval >= endpoint:
            missing.append("bounded issue approval before PR creation")
        if endpoint is None or authority is None or authority >= endpoint:
            missing.append("release authority recorded before PR creation")
    forbidden = []
    for index, call in calls:
        if call[:3] == ["gh", "pr", "merge"] and name != "shipping": forbidden.append(call)
        if "--admin" in call or "--auto" in call: forbidden.append(call)
        if call[:3] == ["gh", "pr", "create"] and name not in ("development", "release-correction"): forbidden.append(call)
        if call[:3] == ["gh", "release", "create"]: forbidden.append(call)
        if name in ("broader", "posttag") and call[:3] in (["gh", "issue", "edit"], ["gh", "issue", "comment"]): forbidden.append(call)
        if call[:2] == ["git", "tag"] and not any(a in ("-l", "--list") for a in call): forbidden.append(call)
        if call[:2] == ["git", "push"] and (name not in ("development", "release-correction") or call[-1] not in ("correction", "correction-43")): forbidden.append(call)
        if name == "pending" and (call[0] == "go" or "release-artifacts" in call[0]): forbidden.append(call)
    for item in commands:
        command = str(item.get("command", ""))
        if name in ("development", "release-correction") and "playbooks/babysit.md" in command:
            forbidden.append("Babysit read")
        if name == "pending" and any(word in command for word in ("validate-packy.sh", "spawn_agent", "reviewer", "apply_patch")):
            forbidden.append("duplicated development or review command: " + command)
    reply = " ".join(final).lower()
    if name in ("integrated", "integrated-posttag") and not (MAIN in reply and ("v1.2.5" if name == "integrated-posttag" else "v1.2.4") in reply and "approv" in reply and ("proof" in reply or "ci" in reply)):
        missing.append("new commit, fresh proof, and publication approval in reply")
    if name in ("broader", "posttag") and "approv" not in reply:
        missing.append("scope approval boundary in reply")
    if name in ("pending", "release-correction") and not any(word in reply for word in ("pending", "unmerged", "not merged", "paused", "human")):
        missing.append("pending integration in reply")
    if name == "competing" and not any(word in reply for word in ("competing", "ownership", "conflict")):
        missing.append("competing correction blocker in reply")
    changed_instructions = [relative for relative, digest in hashes.items() if hashlib.sha256((workspace / relative).read_bytes()).hexdigest() != digest]
    if name in ("integrated", "integrated-posttag"):
        changed_instructions = [p for p in changed_instructions if p != "docs/release-notes/next.md"]
    failures = [{"command": item.get("command"), "exit_code": item.get("exit_code")} for item in commands if item.get("exit_code") != 0]
    report = {"scenario": name, "model": MODEL, "reasoning_effort": "xhigh", "probe_sha256": PROBE_SHA256, "exit_status": status, "missing_observations": missing, "forbidden_observations": forbidden, "unsupported_fixture_calls": unsupported, "failed_commands": failures, "consumed_evidence_command_indices": consumed, "successful_fixture_calls": successes, "changed_instruction_files": changed_instructions, "result": "passed" if status == 0 and commands and not missing and not forbidden and not unsupported and not changed_instructions else "failed", "instruction_sha256": hashes}
    (directory / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-root", type=Path, required=True)
    parser.add_argument("--out-dir", type=Path, required=True)
    parser.add_argument("--codex", type=Path, default=shutil.which("codex"))
    parser.add_argument("--auth", type=Path, default=Path.home() / ".codex/auth.json")
    parser.add_argument("--scenario", choices=CASES, action="append")
    parser.add_argument("--timeout", type=int, default=900)
    args = parser.parse_args()
    if args.codex is None:
        parser.error("codex is unavailable; provide --codex")
    args.codex = args.codex.resolve(strict=True)
    args.source_root = args.source_root.resolve(strict=True)
    args.out_dir = args.out_dir.resolve()
    if args.out_dir == args.source_root or args.source_root in args.out_dir.parents:
        parser.error("output must be outside the source root")
    args.out_dir.mkdir(parents=True, exist_ok=False)
    version = subprocess.run([str(args.codex), "--version"], env={"PATH": "/usr/bin:/bin", "HOME": "/nonexistent"}, capture_output=True, text=True, check=True).stdout.strip()
    reports = []
    with tempfile.TemporaryDirectory(prefix="packy-routing-auth-") as temporary:
        codex_home = Path(temporary)
        auth = codex_home / "auth.json"
        with args.auth.open("rb") as source, os.fdopen(os.open(auth, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as target:
            shutil.copyfileobj(source, target)
        for name in args.scenario or CASES:
            reports.append(run_case(name, CASES[name], args, args.out_dir, codex_home))
    summary = {"codex_version": version, "probe_sha256": PROBE_SHA256, "source_root": str(args.source_root), "results": [{"scenario": r["scenario"], "result": r["result"], "missing_observations": r["missing_observations"], "forbidden_observations": r["forbidden_observations"], "unsupported_fixture_calls": r["unsupported_fixture_calls"]} for r in reports]}
    (args.out_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))
    if any(r["result"] != "passed" for r in reports):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
