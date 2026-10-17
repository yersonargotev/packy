#!/usr/bin/env python3
"""Opt-in real-Catalog acceptance for reviewed skill templates.

Uses the supplied reviewed Catalog checkout's bounded candidate runner. All
adaptations and lifecycle state stay in a new disposable output directory.
Ordinary Go tests do not run this networked, real-content scenario.
"""
import argparse
import importlib.util
import json
from pathlib import Path
import shutil
import sys


def load_runner(catalog):
    probes = catalog / "scripts/probes"
    sys.path.insert(0, str(probes))
    spec = importlib.util.spec_from_file_location("catalog_content_probe", probes / "content-probes.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def adapt(source, destination):
    destination.mkdir()
    shutil.copytree(source / "packs", destination / "packs")
    path = destination / "packs/pstack/pack.json"
    manifest = json.loads(path.read_text())
    if manifest["version"] != "4.3.0":
        raise RuntimeError("this reviewed probe requires pstack 4.3.0 as its baseline")
    manifest["version"] = "5.0.0"
    for resource in manifest["resources"]:
        if resource["kind"] != "skill":
            continue
        if resource["id"] in ("tdd", "teach"):
            for binding in resource["bindings"]:
                binding["invocation"] = ("$" if binding["surface"] == "codex" else "/" if binding["surface"] == "claude" else "") + binding["name"]
                binding["capabilities"].append({"type": "skill-template", "skill_template": {"naming": "pack"}})
                binding["capabilities"].sort(key=lambda item: item["type"])
        elif resource["id"] == "poteto-help":
            for binding in resource["bindings"]:
                if binding["surface"] == "codex":
                    binding["capabilities"].append({"type": "skill-template", "skill_template": {"naming": "native"}})
            for variant in resource["variants"]:
                if variant["surface"] == "codex":
                    variant["requires"] = sorted(set(variant.get("requires", resource["requires"]) + ["skill:tdd", "skill:teach"]))
    path.write_text(json.dumps(manifest, indent=2) + "\n")
    changed = []
    for path in (destination / "packs/pstack/skills/codex/poteto-help").rglob("*.md"):
        original = path.read_text()
        rendered = original
        for name in ("tdd", "teach"):
            rendered = rendered.replace("$" + name, "${{packy.skill:" + name + "}}")
            rendered = rendered.replace("../" + name + "/SKILL.md", "../{{packy.skill:" + name + "}}/SKILL.md")
        if rendered != original:
            path.write_text(rendered)
            changed.append(str(path.relative_to(destination)))
    if len(changed) != 3:
        raise RuntimeError("reviewed help reference inventory changed; reconcile the probe adaptation")
    return sorted(changed)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--catalog", type=Path, required=True, help="reviewed local Catalog Project checkout")
    parser.add_argument("--packy", type=Path, required=True, help="implementation executable under test")
    parser.add_argument("--out-dir", type=Path, required=True, help="new owned output directory")
    parser.add_argument("--focus-pair", action="store_true", help="build a two-Pack acceptance catalog instead of all supplied Packs")
    args = parser.parse_args()
    catalog, executable = args.catalog.resolve(strict=True), args.packy.resolve(strict=True)
    root = args.out_dir.resolve()
    root.mkdir()
    module = load_runner(catalog)
    before = module.tree(catalog / "packs")
    env = module.isolated_environment(root)
    evidence = {"report": "skill-template-catalog-acceptance", "official_publication": False,
                "commands": [], "outcomes": [], "result": "failed"}
    probe = module.Probe(executable, root / "adapted-dist", root, env, evidence, 60)
    try:
        commit, _ = probe.execute(["git", "-C", str(catalog), "rev-parse", "HEAD"])
        version, _ = probe.execute([str(executable), "version", "--json"])
        evidence["builder"] = json.loads(version)
        evidence["catalog_commit"] = commit.strip()
        baseline_source = catalog
        if args.focus_pair:
            baseline_source = root / "baseline-catalog"
            (baseline_source / "packs").mkdir(parents=True)
            for pack in ("matty", "pstack"):
                shutil.copytree(catalog / "packs" / pack, baseline_source / "packs" / pack)
        evidence["catalog_scope"] = "matty-pstack-pair" if args.focus_pair else "complete-catalog"
        adapted = root / "adapted-catalog"
        evidence["adapted_help_sources"] = adapt(baseline_source, adapted)
        probe.execute(["git", "init", "-q", str(adapted)])
        probe.execute(["git", "-C", str(adapted), "add", "packs"])
        probe.execute(["git", "-C", str(adapted), "-c", "user.name=Packy acceptance",
                       "-c", "user.email=packy-acceptance@example.invalid", "commit", "-q", "-m",
                       "Record isolated reviewed skill-template adaptation"])
        adapted_commit, _ = probe.execute(["git", "-C", str(adapted), "rev-parse", "HEAD"])
        evidence["adapted_catalog_commit"] = adapted_commit.strip()
        baseline = module.build_catalog(probe, baseline_source, commit.strip(), root / "baseline-dist")
        candidate = module.build_catalog(probe, adapted, adapted_commit.strip(), probe.snapshot)
        evidence.update(candidate)
        evidence["baseline_archive_sha256"] = baseline["archive_sha256"]
        original = module.Probe(executable, root / "baseline-dist", root, env, evidence, 60)
        for surface in module.SURFACES:
            for scope in ("project", "global"):
                probe.set_context("skill-template-acceptance", surface=surface, scope=scope)
                original.set_context("skill-template-negative-control", surface=surface, scope=scope)
                workspace = original.prepare(surface + "-" + scope)
                apply = "install" if scope == "project" else "activate"
                original.apply(workspace, [apply, "matty", "--surface", surface])
                original.candidate(workspace, [apply, "pstack", "--surface", surface, "--dry-run", "--json"], failure=True)
                original.candidate(workspace, [apply, "pstack", "--surface", surface, "--alias", "skill:tdd=pstack-tdd",
                                               "--alias", "skill:teach=pstack-teach", "--dry-run", "--json"], failure=True)
                installed_before = {name: module.tree(workspace / name) for name in ("home", "config", "project")}
                probe.candidate(workspace, ["transition", "--expect-archive-sha256", baseline["archive_sha256"], "--json"])
                module.require(installed_before == {name: module.tree(workspace / name) for name in installed_before},
                               "catalog transition changed an installed Pack")
                base = workspace / ("project" if scope == "project" else "home")
                skills = base / module.SKILL_DIRECTORIES[surface]
                if scope == "global" and surface == "opencode":
                    skills = workspace / "config/opencode/skills"
                legacy = {name: module.tree(skills / name) for name in ("tdd", "teach")}
                probe.apply(workspace, [apply, "pstack", "--surface", surface])
                for name in ("tdd", "teach"):
                    target = skills / ("pstack-" + name)
                    module.require(target.is_dir() and not target.is_symlink(), "template was not materialized")
                    body = (target / "SKILL.md").read_text()
                    module.require("name: pstack-" + name in body, "wrong native template identity")
                    module.require(module.tree(skills / name) == legacy[name], "Matty definition changed")
                if surface == "codex":
                    body = (skills / "poteto-help/SKILL.md").read_text()
                    for expected in ("$pstack-tdd", "$pstack-teach", "../pstack-tdd/SKILL.md", "../pstack-teach/SKILL.md"):
                        module.require(expected in body, "wrong same-Pack help route: " + expected)
                    for path in (skills / "poteto-help").rglob("*.md"):
                        module.require("{{packy.skill:" not in path.read_text(), "unrendered help directive")
                if scope == "project":
                    probe.verify(workspace)
                # Leave the successful state available for optional real-host discovery probes.
                evidence["outcomes"].append({"surface": surface, "scope": scope, "result": "passed",
                                              "skills": str(skills), "workspace": str(workspace),
                                              "checks": ["legacy-block", "alias-block", "read-only-transition", "full-coexistence",
                                                         "native-frontmatter", "same-pack-references", "legacy-preservation"]})
        module.require(module.tree(catalog / "packs") == before, "supplied Catalog Project changed")
        evidence["result"] = "passed"
    finally:
        (root / "evidence.json").write_text(json.dumps(evidence, indent=2) + "\n")
    print(json.dumps({key: value for key, value in evidence.items() if key not in ("commands",)}, indent=2))


if __name__ == "__main__":
    main()
