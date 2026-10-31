#!/usr/bin/env python3
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
HELPER = ROOT / "scripts/check-evidence-retention.py"
HEADER = "ts\tphase\tdecision\twhy\tevidence\tresult\n"
NAMES = ("probe-v4.log", "probe-v8.log", "artifact-validation.log", "formula.rb", "review.txt", "later-acceptance.log")


def snapshot(root):
    return {str(p.relative_to(root)): (p.stat().st_mode, hashlib.sha256(p.read_bytes()).hexdigest()) for p in root.rglob('*') if p.is_file()}


def main():
    assert HELPER.is_file(), "missing retention helper"
    helper = HELPER
    helper_bytes = helper.read_bytes()
    with tempfile.TemporaryDirectory(prefix="retention-probe-") as tmp:
        root = Path(tmp).resolve()
        env = dict(os.environ)
        for key, name in (("HOME", "home"), ("XDG_CONFIG_HOME", "config"), ("TMPDIR", "tmp")):
            (root / name).mkdir()
            env[key] = str(root / name)
        target, retained, unrelated = (root / x for x in ("owned", "retained", "unrelated-worktree"))
        for path in (target, retained, unrelated):
            path.mkdir()
        (unrelated / "sentinel").write_text("operator state")
        trail = retained / "decisions.tsv"
        originals = [target / name for name in NAMES]
        for path in originals:
            path.write_text(path.name)
        trail.write_text(HEADER + "".join(f"now\tproof\tchecked\twhy\t{p}\tok\n" for p in originals))
        shutil.copyfile(originals[-1], retained / NAMES[-1])
        def relocate(source, dest):
            with trail.open("a") as f:
                f.write(f"now\trelocation\tretained\twhy\t[{source}]({dest})\tok\n")
        relocate(originals[-1], retained / NAMES[-1])
        def check(expect, targets=(target,), handoffs=(), base=None, trail_arg=None):
            args = [sys.executable, "-B", str(helper), "--base", str(base or root), "--trail", str(trail_arg or trail)]
            args += sum((["--target", str(t)] for t in targets), [])
            args += sum((["--handoff", str(h)] for h in handoffs), []) if handoffs else ["--no-handoff"]
            before = snapshot(root)
            process = subprocess.run(args, cwd=root, env=env, capture_output=True, text=True, timeout=30)
            status, out, err = process.returncode, process.stdout, process.stderr
            assert (status == 0) == expect, (args, status, out, err)
            assert snapshot(root) == before, "preflight mutated files"
            return out, err
        def proof(source):
            digest = hashlib.sha256(source.read_bytes()).hexdigest()
            return f"Verified SHA-256 {digest}: {source} -> {retained / source.name}"
        out, err = check(False)
        message = out + err
        for name in NAMES[:-1]:
            assert name in message, message
        assert proof(originals[-1]) in out, out
        for source in originals[:-1]:
            shutil.copyfile(source, retained / source.name)
            relocate(source, retained / source.name)
        out, _ = check(True)
        for source in originals:
            assert proof(source) in out, out
        preserved_proof = proof(originals[0])
        handoff = retained / "handoff.md"
        handoff.write_text(f"[proof]({originals[0]}) and `{originals[1]}`\n")
        check(True, handoffs=(handoff,))
        missing = originals[0]
        pair = f": {missing} -> {retained / missing.name}"
        missing.rename(root / "saved")
        out, _ = check(False)
        assert pair not in out, out
        assert proof(originals[-1]) in out, out
        (root / "saved").rename(missing)
        dest = retained / NAMES[0]
        dest.rename(root / "saved")
        out, _ = check(False)
        assert pair not in out, out
        (root / "saved").rename(dest)
        dest.write_text("wrong bytes")
        out, _ = check(False)
        assert pair not in out, out
        shutil.copyfile(missing, dest)
        check(False, targets=(target, retained / NAMES[0]))
        check(True, targets=(target, root / "absent"))
        check(False, targets=(root / "absent",), handoffs=(retained / "missing.md",))
        missing.rename(root / "saved")
        check(False, targets=(root / "absent",))
        (root / "saved").rename(missing)
        original_trail = trail.read_text()
        for evidence in (str(target), "https://example.com/other; " + str(missing),
                         "${RUN}/proof.log", "[broken](/some/path", "[x](file:///tmp/a)"):
            trail.write_text(HEADER + f"now\tproof\tchecked\twhy\t{evidence}\tok\n")
            check(False)
        trail.write_text(original_trail)
        for text in ("[bad][reference]", "`opaque`", "`$file`", "<a href='proof.log'>proof</a>"):
            handoff.write_text(text + "\n")
            check(False, handoffs=(handoff,))
        handoff.write_text("[relative](later-acceptance.log) and `later-acceptance.log:1`\n")
        check(True, handoffs=(handoff,))
        trail.write_text(HEADER + "now\tproof\tchecked\twhy\tretained/later-acceptance.log:1; retained/review.txt, [result](retained/formula.rb)\tok\n")
        check(True)
        trail.write_text(original_trail)
        (retained / "directory").mkdir()
        raw_bad = (f"{retained}/missing/../{NAMES[0]}",
                   f"{retained}/{NAMES[1]}/../{NAMES[0]}",
                   f"{retained}/{NAMES[0]}/")
        raw_good = f"{retained}/directory/../{NAMES[0]}"
        for cited in raw_bad:
            trail.write_text(HEADER + f"now\tproof\tchecked\twhy\t{cited}\tok\n")
            check(False, targets=(root / "absent",))
            trail.write_text(HEADER)
            handoff.write_text(f"[proof]({cited})\n")
            check(False, targets=(root / "absent",), handoffs=(handoff,))
            trail.write_text(HEADER + f"now\trelocation\tretained\twhy\t[{cited}]({retained / NAMES[0]})\tok\n")
            check(False, targets=(root / "absent",))
            trail.write_text(HEADER + f"now\trelocation\tretained\twhy\t[{retained / NAMES[0]}]({cited})\tok\n")
            check(False, targets=(root / "absent",))
        trail.write_text(HEADER + f"now\tproof\tchecked\twhy\t{raw_good}\tok\n")
        check(True, targets=(root / "absent" / "tail",))
        handoff.write_text(f"[proof]({raw_good})\n")
        check(True, targets=(root / "absent" / "tail",), handoffs=(handoff,))
        trail.write_text(HEADER + f"now\trelocation\tretained\twhy\t[{raw_good}]({originals[0]})\tok\n")
        check(True, targets=(root / "absent",))
        trail.write_text(HEADER + f"now\trelocation\tretained\twhy\t[{originals[0]}]({raw_good})\tok\n")
        check(True, targets=(root / "absent",))
        trail.write_text(HEADER)
        check(True, targets=(root / "absent",), base=f"{retained}/directory/..",
              trail_arg=f"{retained}/directory/../{trail.name}",
              handoffs=(f"{retained}/directory/../{handoff.name}",))
        for bad in (f"{root}/missing/../retained", f"{retained / NAMES[0]}/../retained", f"{retained / NAMES[0]}/"):
            check(False, targets=(root / "absent",), base=bad)
        for bad in (f"{retained}/missing/../{trail.name}", f"{retained / NAMES[0]}/../{trail.name}", f"{trail}/"):
            check(False, targets=(root / "absent",), trail_arg=bad)
        for bad in (f"{retained}/missing/../{handoff.name}", f"{retained / NAMES[0]}/../{handoff.name}", f"{handoff}/"):
            check(False, targets=(root / "absent",), handoffs=(bad,))
        for bad in (f"{root}/missing/../absent", f"{retained / NAMES[0]}/../absent", f"{retained / NAMES[0]}/"):
            check(False, targets=(bad,))
        check(True, targets=(f"{retained}/directory/../../absent/tail",))
        (retained / "review%2Etxt").write_text("literal percent path")
        trail.write_text(HEADER + "now\tproof\tchecked\twhy\t[result](retained/review%2Etxt)\tok\n")
        check(False, targets=(root / "absent",))
        trail.write_text(HEADER)
        handoff.write_text("[result](review%2Etxt)\n")
        check(False, targets=(root / "absent",), handoffs=(handoff,))
        trail.write_text(original_trail)
        link = root / "alias"
        link.symlink_to(target, target_is_directory=True)
        trail.write_text(HEADER + f"now\tproof\tchecked\twhy\t{link}/../owned/{NAMES[0]}\tok\n")
        check(False)
        link.unlink()
        trail.write_text(original_trail)
        relocate(originals[0], retained / NAMES[1])
        check(False)
        trail.write_text(original_trail)
        check(False, targets=(target, retained))
        relocate(originals[0], Path("https://example.com/proof"))
        check(False)
        trail.write_text(original_trail)
        relocate(retained / NAMES[0], retained / NAMES[1])
        check(False)
        trail.write_text(original_trail)
        alias_target = root / target.name.upper()
        aliases = ["/" + str(target)]
        if alias_target.exists() and os.path.samefile(alias_target, target):
            aliases.append(str(alias_target))
            print("case alias supported by filesystem")
        else:
            print("case alias not supported by filesystem; case check skipped")
        owned_handoff = target / "handoff.md"
        owned_handoff.write_text("[proof](../retained/review.txt)\n")
        for alias in aliases:
            trail.write_text(HEADER + f"now\tproof\tchecked\twhy\t{alias}/{NAMES[0]}\tok\n")
            check(False)
            trail.write_text(HEADER + f"now\tproof\tchecked\twhy\t{retained / NAMES[0]}\tok\n")
            relocate(retained / NAMES[0], f"{alias}/{NAMES[0]}")
            check(False)
            trail.write_text(original_trail)
            check(False, handoffs=(f"{alias}/handoff.md",))
        assert helper.read_bytes() == helper_bytes
        assert (unrelated / "sentinel").read_text() == "operator state"
        originals[0].unlink()
        retained_hash = hashlib.sha256((retained / NAMES[0]).read_bytes()).hexdigest()
        assert preserved_proof == f"Verified SHA-256 {retained_hash}: {originals[0]} -> {retained / NAMES[0]}"
        print(f"{helper.relative_to(ROOT)}: partial copy blocked; verified copies passed; negatives and read-only state verified")


if __name__ == "__main__":
    main()
