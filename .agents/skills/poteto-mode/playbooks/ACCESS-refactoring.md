> For on-demand use, read `../ACCESS.md` first and carry its initiating
> surface/snapshot, target, prepared resource paths and writable output context.
> Resolve playbook paths from the returned resource root.

> Apply the parent skill's host execution contract. Report unavailable
> scheduling, independent review, or live verification as incomplete gates.
> Keep external actions within the user's authorization.

### Refactoring

**You own the contract. The structure changes. The behavior does not.** Distinct from Feature, which adds behavior, and Bug fix, which corrects it.

If the cleanup reveals a missing feature or a real bug, split it out and ship the structural change first against the pinned contract. A redesign is allowed, but name it and route to Feature. Large or cross-cutting structural work belongs to the optional `pstack:figure-it-out` route (retrieve it with the initiating surface/snapshot when selected; if unavailable, design and verify a scoped direct workflow). This playbook is the focused-to-medium change.

1. Pin the behavior contract first. Run the [pstack:how]({{packy.access:pstack:how}}) skill over the affected subsystem to learn the contract, then write a characterization test, snapshot, or equivalence harness that captures current behavior before any structure moves. If the area has no coverage, write the pin before touching structure. Type check and lint are not a pin.
2. Name the structure the code is missing per [pstack:principle-model-the-domain]({{packy.access:pstack:principle-model-the-domain}}). Boring code stays when the shape is already clear and local. The reshape must delete branches or invalid states, not add indirection.
3. Name the target shape. State what the module layout, types, and call graph should be if built today ([pstack:principle-foundational-thinking]({{packy.access:pstack:principle-foundational-thinking}}), [pstack:principle-redesign-from-first-principles]({{packy.access:pstack:principle-redesign-from-first-principles}})). If the target crosses a function boundary, run the [pstack:architect]({{packy.access:pstack:architect}}) skill for parallel design exploration of the shape before the move.
4. Subtract before you add. Delete dead code, collapse one-caller wrappers, drop redundant validators, and remove orphan references before introducing the new shape ([pstack:principle-subtract-before-you-add]({{packy.access:pstack:principle-subtract-before-you-add}})). The smallest change that reaches the target shape ships ([pstack:principle-laziness-protocol]({{packy.access:pstack:principle-laziness-protocol}})). A speculative cleanup that "might help" gets reverted.
5. Move in small behavior-preserving steps, each keeping the pin green. For API reshapes, migrate every caller and delete the old API in the same wave ([pstack:principle-migrate-callers-then-delete-legacy-apis]({{packy.access:pstack:principle-migrate-callers-then-delete-legacy-apis}})). No compatibility shims, no parallel old-and-new paths. Spot-check every rename against the actual files. Renames silently miss usages in strings, prose, and back-references. Delegate the mechanical edits to a subagent using your configured refactoring model (default `inherit-parent`) with a specific scope (file paths, the names being moved, the behavior to hold). Match reasoning to task difficulty with supported controls and confirmed role preferences. Use standard speed unless urgency calls for fast mode; verification preserves its configured speed. For `auto` or `inherit-parent`, omit model, effort, and speed overrides.
6. Prove behavior is unchanged on the real artifact, not "it compiles" ([pstack:principle-prove-it-works]({{packy.access:pstack:principle-prove-it-works}})). For larger reshapes, run an equivalence check: a script that diffs old-vs-new outputs, a recorded baseline replayed against the new code, or a smoke run on the matching surface via the relevant control skill.
7. Confirm the change is worth keeping. The success measure is reduced reader load ([pstack:principle-minimize-reader-load]({{packy.access:pstack:principle-minimize-reader-load}})). If the diff does not lower reader load somewhere, revert it.
8. Rebase into small ordered commits. A subtraction commit, then the reshape, then any follow-on cleanup. Shape them with the [pstack:principle-sequence-verifiable-units]({{packy.access:pstack:principle-sequence-verifiable-units}}) principle skill, so each behavior-preserving slice stays green before the next. Run **Opening a PR**.

**Reply:** the structure that changed, the pin you held it against, the equivalence proof, the reader-load delta, what shipped and what got reverted. No new behavior.
