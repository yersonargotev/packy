# Human plan — real Matty dry run

## Outcome

`blocked` for candidate `v1.1.0` / `d574778f94cf620fcc8ce741584093bc650a61d3`. Provenance is eligible, but the real bundle has five intentional local modifications and lacks the production source/lock, historical artifact, accepted compatibility evidence, and hardened validation entrypoint required for publication.

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
