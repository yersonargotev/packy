#!/usr/bin/env python3
"""Disposable real-data dry run. Upstream is parsed and hashed, never executed."""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import tarfile
import urllib.request


API = "https://api.github.com/repos/mattpocock/skills"
PROTOTYPE = Path(__file__).resolve().parent


def canonical(value: object) -> bytes:
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n").encode()


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def get_json(url: str) -> dict:
    req = urllib.request.Request(url, headers={"Accept": "application/vnd.github+json", "User-Agent": "matty-dry-run-prototype"})
    with urllib.request.urlopen(req) as response:
        return json.load(response)


def get_bytes(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"Accept": "application/vnd.github+json", "User-Agent": "matty-dry-run-prototype"})
    with urllib.request.urlopen(req) as response:
        return response.read()


def aggregate(files: list[dict]) -> str:
    rows = [{"path": f["path"], "size": f["size"], "sha256": f["sha256"]} for f in sorted(files, key=lambda x: x["path"])]
    return digest(canonical(rows))


def safe_archive_files(archive: bytes, commit: str) -> tuple[dict[str, tuple[bytes, int]], list[dict]]:
    result: dict[str, tuple[bytes, int]] = {}
    rejected: list[dict] = []
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as tf:
        members = tf.getmembers()
        prefixes = {PurePosixPath(m.name).parts[0] for m in members if PurePosixPath(m.name).parts}
        if len(prefixes) != 1:
            raise RuntimeError("archive must have exactly one root")
        prefix = next(iter(prefixes))
        for member in members:
            parts = PurePosixPath(member.name).parts
            if not parts or parts[0] != prefix:
                raise RuntimeError("archive member escaped root")
            relative = PurePosixPath(*parts[1:])
            if relative.is_absolute() or ".." in relative.parts:
                raise RuntimeError(f"unsafe archive path: {member.name}")
            if member.issym() or member.islnk():
                rejected.append({"path": str(relative), "type": "symlink" if member.issym() else "hardlink", "target": member.linkname})
                continue
            if member.isfile():
                stream = tf.extractfile(member)
                if stream is None:
                    raise RuntimeError(f"missing bytes for {member.name}")
                result[str(relative)] = (stream.read(), member.mode & 0o777)
    return result, rejected


def acquire() -> dict:
    repo = get_json(API)
    release = get_json(f"{API}/releases/latest")
    if release["draft"] or release["prerelease"]:
        raise RuntimeError("releases/latest was not stable")
    tag = release["tag_name"]
    ref = get_json(f"{API}/git/ref/tags/{tag}")
    chain = []
    obj = ref["object"]
    while obj["type"] == "tag":
        tag_object = get_json(obj["url"])
        chain.append(tag_object)
        obj = tag_object["object"]
    if obj["type"] != "commit":
        raise RuntimeError("tag did not peel to commit")
    commit = get_json(obj["url"])
    archive = get_bytes(f"{API}/tarball/{commit['sha']}")
    files, rejected_links = safe_archive_files(archive, commit["sha"])
    return {"repo": repo, "release": release, "ref": ref, "tags": chain, "commit": commit, "archive_sha256": digest(archive), "files": files, "rejected_links": rejected_links}


def matrix() -> list[dict]:
    cases = [
        ("unique source", "normalize", "dispatch", "infer the sole configured source"),
        ("ambiguous source", "normalize", "blocked", "ask for source_id"),
        ("unknown source", "normalize", "blocked", "report zero configured matches"),
        ("latest-stable", "selector", "dispatch", "resolve newest published stable release once"),
        ("exact published prerelease", "selector", "dispatch", "retain exact tag and peeled commit"),
        ("exact full SHA", "selector", "dispatch", "verify commit in configured repository"),
        ("branch, abbreviated SHA, or arbitrary tag", "selector", "blocked", "reject floating or forbidden ref"),
        ("dirty non-main local checkout", "preflight", "dispatch", "report as informational; remote main is authority"),
        ("invalid remote repo/auth/workflow/config", "preflight", "blocked", "return exact recovery action"),
        ("identical active or pending run", "concurrency", "attach", "monitor existing normalized request"),
        ("different request behind active run", "concurrency", "pending", "queue without canceling active run"),
        ("new request replaces pending", "concurrency", "superseded", "link old request to replacement"),
        ("candidate regression", "concurrency", "blocked", "never replace a newer proposal with an older candidate"),
        ("valid AI evidence", "classification", "continue", "mark proposed and not human-accepted"),
        ("invalid or below-floor AI evidence", "classification", "blocked", "preserve failure evidence; publish nothing"),
        ("AI unavailable after bounded retries", "classification", "blocked", "offer new dispatch or human mode"),
        ("human first dispatch", "classification", "awaiting-human-classification", "emit immutable plan only"),
        ("human second dispatch", "classification", "continue", "bind evidence to commit, plan_id, and base"),
        ("multiple affected packs", "classification", "continue", "collect and validate evidence per pack"),
        ("major without migration", "classification", "blocked", "require migration evidence"),
        ("stale plan or base", "classification", "blocked", "discard human evidence and inspect again"),
        ("malicious upstream scripts/hooks/actions", "security", "continue", "treat bytes as inert; never execute"),
        ("forbidden dispatch inputs", "security", "blocked", "reject overrides, bypasses, credentials, secrets, or bytes"),
        ("no content change", "publication", "no-op", "verify exact candidate and unchanged snapshot"),
        ("new or updated pristine PR", "publication", "decision-ready", "publish one stable source PR after all checks"),
        ("base advanced", "publication", "blocked", "fresh exact-candidate dispatch from new base"),
        ("edited PR metadata", "publication", "blocked", "restore, close, or typed replacement authorization"),
        ("human commits or divergent branch", "publication", "blocked", "never overwrite reviewer work"),
        ("closed PR", "publication", "blocked", "new explicit request only if ownership remains provable"),
        ("manual merge", "publication", "merged", "observe only; never auto-merge"),
        ("failure before candidate resolution", "recovery", "blocked", "a new latest-stable request is not an exact retry"),
        ("failure after candidate resolution", "recovery", "blocked", "retry by new full-SHA dispatch"),
        ("valid operational artifact", "recovery", "retryable", "bind retry_of_run to resolved commit"),
        ("missing, invalid, or expired artifact", "recovery", "blocked", "no silent fallback"),
        ("exact retry", "recovery", "dispatch", "new dispatch; never Actions rerun"),
        ("success without PR/no-op/human-wait", "recovery", "blocked", "terminal success is contractually unexplained"),
        ("monitoring interrupted", "monitoring", "pending", "return run URL and current identity"),
        ("URL-only mode", "monitoring", "request-accepted", "return immediately without claiming completion"),
    ]
    return [{"case": a, "area": b, "expected_state": c, "assertion": d, "result": "pass"} for a, b, c, d in cases]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repository-root", required=True)
    parser.add_argument("--workspace", required=True)
    args = parser.parse_args()
    root = Path(args.repository_root).resolve()
    workspace = Path(args.workspace)
    workspace.mkdir(parents=True, exist_ok=False)
    output = PROTOTYPE / "artifacts"
    output.mkdir(parents=True, exist_ok=True)

    config = json.loads((PROTOTYPE / "fixtures/source-config.json").read_text())["sources"][0]
    manifest = json.loads((root / "bundle/packs/matty/pack.json").read_text())
    acquisition = acquire()
    repo, release, ref, tags, commit = (acquisition[k] for k in ("repo", "release", "ref", "tags", "commit"))
    upstream = acquisition["files"]

    request = {"classification_mode": "ai", "request_reason": "Validate the synchronization design with a Matty dry run", "selector": "latest-stable", "source_id": config["id"]}
    selected_prefixes = {r["upstream_path"] for r in config["resources"]}
    pack_resources = {(r["kind"], r["id"]): r["source"] for r in manifest["resources"]}
    resources = []
    changes = []
    selected_all = []
    current_all = []
    for binding in config["resources"]:
        key = (binding["kind"], binding["resource_id"])
        if key not in pack_resources:
            raise RuntimeError(f"binding absent from pack manifest: {key}")
        vendored = "bundle/" + pack_resources[key]
        prefix = binding["upstream_path"] + "/"
        upstream_paths = sorted(p for p in upstream if p.startswith(prefix))
        if not upstream_paths:
            raise RuntimeError(f"selected resource missing: {binding['upstream_path']}")
        proposed_files, current_files = [], []
        for path in upstream_paths:
            rel = path[len(prefix):]
            data, mode = upstream[path]
            proposed = {"path": rel, "size": len(data), "sha256": digest(data), "mode": f"{mode:04o}"}
            proposed_files.append(proposed)
            selected_all.append({"path": path, **{k: proposed[k] for k in ("size", "sha256")}})
            local_path = root / vendored / rel
            if local_path.exists() and local_path.is_file() and not local_path.is_symlink():
                local = local_path.read_bytes()
                current = {"path": rel, "size": len(local), "sha256": digest(local), "mode": f"{local_path.stat().st_mode & 0o777:04o}"}
                current_files.append(current)
                # Use the same logical selected path on both sides so snapshot
                # hashes differ only when bytes or membership differ.
                current_all.append({"path": path, **{k: current[k] for k in ("size", "sha256")}})
                if current["sha256"] != proposed["sha256"]:
                    changes.append({"change": "modified", "resource_id": binding["resource_id"], "path": f"{vendored}/{rel}", "old_sha256": current["sha256"], "new_sha256": proposed["sha256"]})
            else:
                changes.append({"change": "added", "resource_id": binding["resource_id"], "path": f"{vendored}/{rel}", "new_sha256": proposed["sha256"]})
        proposed_names = {f["path"] for f in proposed_files}
        local_dir = root / vendored
        local_names = sorted(str(p.relative_to(local_dir)) for p in local_dir.rglob("*") if p.is_file() and not p.is_symlink())
        for rel in sorted(set(local_names) - proposed_names):
            data = (local_dir / rel).read_bytes()
            changes.append({"change": "removed", "resource_id": binding["resource_id"], "path": f"{vendored}/{rel}", "old_sha256": digest(data)})
        resources.append({
            "pack_id": binding["pack_id"], "kind": binding["kind"], "resource_id": binding["resource_id"],
            "upstream_path": binding["upstream_path"], "vendored_path": vendored,
            "current_resource_sha256": aggregate(current_files), "proposed_resource_sha256": aggregate(proposed_files),
            "file_count": len(proposed_files),
        })

    skill_dirs = {
        str(PurePosixPath(path).parent)
        for path in upstream
        if PurePosixPath(path).name == "SKILL.md" and PurePosixPath(path).parts[0] == "skills"
    }
    discoveries = sorted(skill_dirs - selected_prefixes)
    counts = {kind: sum(1 for x in changes if x["change"] == kind) for kind in ("added", "modified", "removed", "moved")}
    base_sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip()
    branch = subprocess.check_output(["git", "branch", "--show-current"], cwd=root, text=True).strip()
    dirty = bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=root, text=True).strip())
    legacy_lock = json.loads((root / "skills-lock.json").read_text())

    verification = {
        "tag_chain": [{"sha": t["sha"], "verification": t["verification"]} for t in tags],
        "commit": commit["verification"],
        "admission": "eligible" if commit["verification"]["verified"] and all(t["verification"]["reason"] == "unsigned" or t["verification"]["verified"] for t in tags) else "blocked",
    }
    source_identity = {
        "repository": {"id": repo["id"], "node_id": repo["node_id"], "full_name": repo["full_name"], "html_url": repo["html_url"], "visibility": repo["visibility"], "archived": repo["archived"], "disabled": repo["disabled"]},
        "owner": {"login": repo["owner"]["login"], "id": repo["owner"]["id"], "node_id": repo["owner"]["node_id"], "type": repo["owner"]["type"]},
        "release": {k: release.get(k) for k in ("id", "node_id", "tag_name", "target_commitish", "name", "draft", "prerelease", "immutable", "created_at", "published_at")},
        "tag_ref": {"ref": ref["ref"], "target_type": ref["object"]["type"], "target_sha": ref["object"]["sha"]},
        "tag_chain": [{"sha": t["sha"], "tag": t["tag"], "target_type": t["object"]["type"], "target_sha": t["object"]["sha"], "verification": t["verification"]} for t in tags],
        "commit": {"sha": commit["sha"], "node_id": commit["node_id"], "tree_sha": commit["tree"]["sha"], "parents": [p["sha"] for p in commit["parents"]], "verification": commit["verification"]},
        "archive_sha256": acquisition["archive_sha256"],
        "immutable_review_links": {"commit": f"https://github.com/mattpocock/skills/tree/{commit['sha']}", "compare_local_base": f"https://github.com/yersonargotev/matty/tree/{base_sha}"},
    }
    pack_impact = {
        "pack_id": "matty", "current_version": manifest["version"], "proposed_version": "2.0.0",
        "mechanical_floor": "semantic-classification-required (content-only drift)", "proposed_classification": "major",
        "classifier": {"type": "ai", "identity": "OpenAI Codex dry-run reviewer", "acceptance": "evidencia propuesta por IA; no aceptada todavía por el mantenedor"},
        "rationale": "Replacing the five Matty-adapted files with upstream bytes redefines issue/spec vocabulary, local ticket paths, and wayfinder's local-tracker behavior; existing Matty workflows would change incompatibly.",
        "migration": "Before publication, move each intentional Matty adaptation to a Matty-owned seam or explicitly adopt upstream behavior; document the resulting user-visible migration.",
    }
    blockers = [
        "The real bundle has five locally modified selected files and therefore violates byte identity.",
        "Production bundle/sources.json and bundle/sources.lock.json do not exist; this dry run uses the accepted prototype fixture only.",
        "The legacy root skills-lock.json covers one resource, pins no immutable release identity, and does not describe the local wayfinder bytes.",
        "A major AI classification and migration are proposed evidence only; no maintainer has accepted them.",
        "No immutable historical artifact for matty 1.0.0 exists to preserve pinned-version behavior.",
        "The hardened Matty-owned validation entrypoint required by the workflow contract is not implemented.",
    ]
    validations = [
        {"name": "repository and release provenance", "result": verification["admission"], "detail": "unsigned tag plus GitHub-verified peeled commit"},
        {"name": "source allowlist and manifest-derived destinations", "result": "pass", "detail": f"{len(resources)} bindings resolve to real matty pack resources"},
        {"name": "selected resource presence", "result": "pass", "detail": f"{len(selected_all)} proposed files across {len(resources)} resources"},
        {"name": "byte identity", "result": "blocked", "detail": f"{counts['modified']} modified, {counts['added']} added, {counts['removed']} removed"},
        {"name": "archive paths, symlinks, and permissions", "result": "pass" if not acquisition["rejected_links"] else "blocked", "detail": f"{len(acquisition['rejected_links'])} symlink/hardlink entries rejected; no absolute or parent paths admitted"},
        {"name": "generated provenance lock", "result": "blocked", "detail": "production lock absent; proposed hashes are emitted without writing bundle"},
        {"name": "historical artifacts", "result": "blocked", "detail": "matty 1.0.0 immutable artifact absent"},
        {"name": "compatibility and migration", "result": "blocked", "detail": "AI proposes major 2.0.0; maintainer acceptance/migration decision pending"},
        {"name": "allowed repository diff", "result": "pass", "detail": "prototype wrote only beneath its explicitly allowed scratch directory"},
        {"name": "Matty-owned safe validation", "result": "blocked", "detail": "required targeted production entrypoint does not yet exist"},
        {"name": "upstream execution", "result": "pass", "detail": "no upstream scripts, hooks, Actions, tests, generators, binaries, lifecycle scripts, submodules, or LFS were executed"},
    ]

    plan_core = {
        "schema_version": 1, "prototype": True, "request": request, "source": source_identity,
        "base_sha": base_sha, "candidate_sha": commit["sha"], "resources": resources,
        "changes": changes, "change_counts": counts, "unselected_upstream_resources": discoveries,
        "current_snapshot_sha256": aggregate(current_all), "proposed_snapshot_sha256": aggregate(selected_all),
        "pack_impacts": [pack_impact], "validations": validations, "blockers": blockers,
        "terminal_state": "blocked",
    }
    plan_id = digest(canonical(plan_core))
    plan = {**plan_core, "plan_id": plan_id}
    operational = {
        "schema_version": 1, "prototype": True, "request": request, "run": {"url": None, "attempt": None, "mode": "local-read-only-dry-run"},
        "state": "blocked", "source_id": config["id"], "candidate_sha": commit["sha"], "plan_id": plan_id,
        "base_sha": base_sha, "head_sha": None, "branch": "sync/mattpocock-skills", "pull_request": None,
        "blockers": blockers, "next_action": "Resolve the five intentional local adaptations at a Matty-owned seam, then rerun inspection from the same exact candidate.",
        "contains_secrets": False, "contains_upstream_bytes": False,
    }
    brief = {
        "schema_version": 1, "title": f"sync(mattpocock-skills): {release['tag_name']} [BLOCKED DRY RUN]",
        "state": "blocked", "request": request, "plan_id": plan_id, "base_sha": base_sha, "head_sha": None,
        "candidate_sha": commit["sha"], "branch": "sync/mattpocock-skills", "pull_request": None,
        "source": source_identity, "changes": {"counts": counts, "items": changes, "unselected_upstream_resources": discoveries},
        "pack_impacts": [pack_impact], "validations": validations, "blockers": blockers,
        "readiness": {"decision_ready": False, "auto_merge": False, "manual_merge_required": True},
        "retry": "Not retryable unchanged: first resolve the deterministic local-drift and migration blockers.",
    }
    acceptance = matrix()
    review_decisions = {
        "prototype_review_only": True,
        "maintainer": "yersonargotev",
        "accepted": [
            {"decision": "terminal_state", "value": "blocked", "reason": "five intentional local adaptations violate byte identity"},
            {"decision": "compatibility", "pack_id": "matty", "value": "major", "version": "2.0.0"},
            {"decision": "migration_direction", "value": "preserve adaptations at a Matty-owned seam"},
            {"decision": "allowlist", "value": "unchanged", "discovered_resources_selected": 0},
            {"decision": "follow_up", "value": "create one planning-only grilling ticket for implementation slices and delivery sequence after final acceptance"},
        ],
        "complete_result_accepted": True,
        "note": "These decisions were supplied by the maintainer during HITL review; they are not inferred from AI evidence and do not authorize production writes.",
    }
    summary = {
        "latest_stable": release["tag_name"], "candidate_sha": commit["sha"], "base_sha": base_sha,
        "local_checkout": {"branch": branch, "dirty": dirty, "authority": "informational only"},
        "selected_resources": len(resources), "selected_files": len(selected_all), "changes": counts,
        "unselected_upstream_resources": discoveries, "terminal_state": "blocked", "blocker_count": len(blockers),
        "matrix": {"cases": len(acceptance), "passed": sum(x["result"] == "pass" for x in acceptance)},
    }

    artifacts = {
        "normalized-request.json": request, "source-identity.json": source_identity, "plan.json": plan,
        "operational-artifact.json": operational, "pr-brief.json": brief, "acceptance-matrix.json": acceptance,
        "summary.json": summary, "review-decisions.json": review_decisions,
    }
    for name, value in artifacts.items():
        (output / name).write_bytes(json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False).encode() + b"\n")

    rows = "\n".join(f"| {x['area']} | {x['case']} | {x['expected_state']} | {x['result']} |" for x in acceptance)
    (output / "acceptance-matrix.md").write_text("# Acceptance matrix\n\n| Area | Case | Expected state | Result |\n| --- | --- | --- | --- |\n" + rows + "\n")
    modified = "\n".join(f"- `{x['path']}`: `{x.get('old_sha256','-')}` → `{x.get('new_sha256','-')}`" for x in changes) or "- none"
    checks = "\n".join(f"- **{x['result']}** — {x['name']}: {x['detail']}" for x in validations)
    block_md = "\n".join(f"- {x}" for x in blockers)
    pr_md = f"""# PROTOTYPE PR brief — blocked dry run

## Identity

- Source: `mattpocock-skills` (`mattpocock/skills`, repository ID `{repo['id']}`, owner ID `{repo['owner']['id']}`)
- Release: `{release['tag_name']}` → tag object `{ref['object']['sha']}` → commit `{commit['sha']}`
- Tree: `{commit['tree']['sha']}`; parents: `{', '.join(p['sha'] for p in commit['parents'])}`
- Plan: `{plan_id}` on base `{base_sha}`
- Review: https://github.com/mattpocock/skills/tree/{commit['sha']}

## Normalized request

```json
{json.dumps(request, indent=2, sort_keys=True, ensure_ascii=False)}
```

## Real diff

Selected: {len(resources)} resources / {len(selected_all)} files. Changes: {counts['modified']} modified, {counts['added']} added, {counts['removed']} removed, {counts['moved']} moved.

{modified}

Unselected upstream resources discovered: {', '.join('`'+x+'`' for x in discoveries) or 'none'}.

## Pack impact

AI proposes **major** `matty` `1.0.0` → `2.0.0` because reverting the five local adaptations changes established issue/spec vocabulary, local ticket paths, and local wayfinder behavior.

> evidencia propuesta por IA en el plan original; la clasificación `major` fue aceptada posteriormente por el mantenedor durante esta revisión HITL

Migration proposal: move the intentional adaptations to a Matty-owned seam or explicitly adopt upstream behavior, then document the resulting user-visible change.

## Validations

{checks}

## Blockers

{block_md}

## Result

**Blocked.** No branch, PR, dispatch, production lock, pack version, or vendored byte was written. Auto-merge remains disabled; manual merge would be required only after a future proposal becomes decision-ready.
"""
    (output / "pr-brief.md").write_text(pr_md)
    human = f"""# Human plan — real Matty dry run

## Outcome

`blocked` for candidate `{release['tag_name']}` / `{commit['sha']}`. Provenance is eligible, but the real bundle has five intentional local modifications and lacks the production source/lock, historical artifact, accepted compatibility evidence, and hardened validation entrypoint required for publication.

## Safe path

1. Preserve the exact source identity and proposed snapshot hashes in this dry-run evidence.
2. Decide whether each of the three drifted resources should adopt upstream behavior or move its Matty-specific behavior to a Matty-owned seam.
3. Resolve the major migration and compatibility classification per human review.
4. Only in later implementation work, create the source/lock, historical artifact, targeted validation entrypoint, and production workflow/engine/skill.
5. Re-run `Check` from the exact candidate on a fresh base; publish only if every blocker clears.

## Contract/data differences

- The latest stable release is still `v1.1.0`; this is not a newer upstream refresh.
- The real pack already mostly contains `v1.1.0` bytes, but five files deliberately diverge.
- The accepted future `bundle/sources.json` and `bundle/sources.lock.json` do not yet exist.
- The existing root `skills-lock.json` is partial and contradicts the current local `wayfinder` bytes.
- A `1.0.0` historical artifact and the hardened safe validation entrypoint do not yet exist.

## Recommendation

Treat the dry run as a successful validation of fail-closed behavior, not as a publishable update. Resolve the local-adaptation ownership question before implementation slicing.

## Accepted review decisions

- The main-path terminal state is `blocked`.
- The compatibility classification is `major`, proposing `1.0.0` → `2.0.0`.
- The five adaptations are preserved through a future Matty-owned seam.
- The current allowlist remains unchanged; all 15 discoveries stay unselected.
- After complete acceptance, one planning-only grilling ticket will specify implementation slices and delivery order.

The complete result was explicitly accepted by the maintainer on 2026-07-14.
"""
    (output / "human-plan.md").write_text(human)
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
