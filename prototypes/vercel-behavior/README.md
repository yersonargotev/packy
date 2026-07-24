# Vercel lifecycle behavior prototype

> **THROWAWAY PROTOTYPE** — discussion aid only. This is not Packy
> implementation and must never be merged into `main`.

## Question under test

Does one per-surface state model make Vercel activation, update, deactivation,
collision handling, authority disclosure, invocation-time degradation, and
`configured → authorized → usable` readiness understandable without implying
that Packy executes an upstream skill, authenticates, links a project, reads a
token, or deploys during reconciliation?

The model keeps all state in memory. It does not inspect or write `HOME` or
`XDG_CONFIG_HOME`, invoke Packy, execute upstream files, authenticate, access
the network, or mutate a consumer project.

## Confirmed decisions

1. A collision involving any mandatory Vercel skill blocks all nine skills on
   that surface before effects, while other surfaces remain independent.
2. Successful projection completes activation at `configured`; host trust and
   loading advance readiness through `authorized` and `usable`. Missing or
   unverified invocation-mode prerequisites remain separate and do not degrade
   whole-pack readiness.
3. Activation, update, reconciliation, and status may inspect and reconcile
   inert skill trees only. They never execute upstream scripts, authenticate,
   read tokens, link projects, perform skill-directed Git operations, or
   deploy; those authorities belong only to deliberate skill invocation.
4. When `vercel-optimize` cannot fan out to subagents, it remains available
   through its declared sequential-investigation fallback. A missing
   indispensable prerequisite blocks only the requested invocation before
   effects; Packy never invents a fallback or simulates success.
5. Deactivation removes only projections whose Packy ownership and fingerprint
   are freshly verified. Drifted, ambiguous, or unmanaged targets are
   preserved and reported as pending human actions rather than adopted,
   overwritten, or deleted automatically.
6. Update preserves each explicit surface-local alias, replaces all nine skill
   projections atomically, and returns to `configured`. Authorization and host
   loading must be freshly observed before update can claim `authorized` or
   `usable`; prior readiness evidence is not carried forward.
7. Preview and status disclose requirements per capability and mode: tools and
   versions, authentication/linkage/entitlements, possible authorities and
   effects, verified fallback, and pre-effect failure behavior. Secret values
   and token-bearing commands are never displayed; only presence, absence, or
   redacted identity may be reported.

## Run

```sh
python3 prototypes/vercel-behavior/prototype.py
```

Suggested path through the prototype:

1. `collision` then `activate` to see an atomic surface blocker.
2. `alias` then `activate`, followed by `approve`, `trust`, and `load`.
3. `mode optimize` to see invocation availability remain separate from pack
   readiness; use `enable optimize` to exercise the declared sequential
   fallback.
4. `surface opencode` and activate it to see surface independence.
5. `drift`, `reconcile`, and `approve` to see inert repair.
6. `update`, `approve`, `deactivate`, and `approve` to inspect lifecycle
   continuity and cleanup.
