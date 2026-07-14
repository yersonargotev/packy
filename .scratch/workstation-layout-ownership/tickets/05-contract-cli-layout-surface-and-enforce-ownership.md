Status: resolved

# Contract the CLI layout surface and enforce ownership

## What to build

Complete the contraction after every production caller uses owner APIs. Replace
CLI test setup with an owner-assembled fixture, delete the shared production
layout model and all obsolete derivation or mapping helpers, and add structural
enforcement that keeps ambient reads and artifact layout with their approved
owners.

## Blocked by

- [Route capability packs through owning layouts](03-route-capability-packs-through-owning-layouts.md)
- [Route setup health through owner observations](04-route-setup-health-through-owner-observations.md)

## Acceptance criteria

- [x] No production caller depends on the former shared path structure or resolver.
- [x] The shared path type, resolver, default-source helper, mapping helpers, duplicated CLI derivation, and obsolete compatibility wrappers are deleted.
- [x] CLI production code contains no state, skill, host, Installed Source, or Engram candidate path policy.
- [x] Ambient environment, user-home, and current-directory reads are limited to the approved process edge and workstation resolver.
- [x] CLI end-to-end tests use a test-only aggregate fixture assembled exclusively from owner APIs.
- [x] The fixture derives no canonical path independently and is unavailable to production code.
- [x] Obsolete tests protecting the old layout decomposition are removed only after equivalent owner and CLI contracts cover their behavior.
- [x] Positive owner contracts cover normalization, overrides, state roots, sources, skills, hosts, and executable topology.
- [x] Focused structural enforcement prevents reintroducing the shared layout model, known CLI artifact derivation, and unauthorized ambient reads.
- [x] Structural enforcement avoids a fragile repository-wide scan of every path-like literal.
- [x] Help, version, init, install, update, uninstall, pack, and doctor compatibility remain covered at the CLI seam.
- [x] No forwarding facade, compatibility wrapper, or dual layout ownership survives.
- [x] Repository documentation and accepted architecture remain consistent with the final code.
- [x] The complete repository test suite and diff checks pass with sandboxed Home and XDG configuration.

## Out of scope

- User-visible behavior, path, schema, output, or command changes.
- New abstractions unrelated to enforcing workstation layout ownership.
- Opportunistic cleanup outside the obsolete layout surface.
