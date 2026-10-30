> For on-demand use, read `../ACCESS.md` first and carry its initiating
> surface/snapshot, target, prepared resource paths and writable output context.
> Resolve playbook paths from the returned resource root.

> Apply the parent skill's host execution contract. Report unavailable
> scheduling, independent review, or live verification as incomplete gates.
> Keep external actions within the user's authorization.

### Feature

**You own the design. Plan, review, verify.** Delegate implementation. Stay in the lead.

1. [pstack:how]({{packy.access:pstack:how}}) over the affected subsystem.
2. [pstack:architect]({{packy.access:pstack:architect}}) for parallel design exploration.
3. Write the throughput checkpoint as four todo items. A dimension that genuinely does not apply (single file, no fan-out) keeps its item with `n/a: <reason>` rather than being dropped:
   - **Blocking first steps.** Gates run before fan-out.
   - **Independent workstreams.** Disjoint files, services, or layers parallelize. Shared writes serialize.
   - **Shared mutable state.** Default to splitting the target (the [pstack:principle-separate-before-serializing-shared-state]({{packy.access:pstack:principle-separate-before-serializing-shared-state}}) principle skill). Serialize only for real invariants.
   - **Smallest safe decomposition.** If one worker is best, name why.
4. Delegate code-writing to a subagent using your configured feature model (default `inherit-parent`) with a specific scope (file paths, named data shape and its organizing structure per [pstack:principle-model-the-domain]({{packy.access:pstack:principle-model-the-domain}}), a state machine over scattered booleans, a table/registry over branching, a typed model over repeated shape assumptions, chosen before the delegate writes logic, and success criteria). When the implementation admits multiple valid shapes (error handling, abstraction layer, test structure), delegate via the [pstack:arena]({{packy.access:pstack:arena}}) skill instead so the runners surface the alternatives and the cross-judge guards the pick. Mandatory: no skip-with-reason escape, and Laziness Protocol does not override it (the gain is review separation, not lines saved). A subagent forbidden to spawn satisfies this by owning the diff directly with the same review separation. No "standing by" reply that waits on a nested agent. Comments per **Comments**. Surgical edits, re-ground against the source for upstream-derived files. Port shared-primitive improvements to all consumers and verify each. Commit liberally. Match reasoning to task difficulty with supported controls and confirmed role preferences. Use standard speed unless urgency calls for fast mode; verification preserves its configured speed. For `auto` or `inherit-parent`, omit model, effort, and speed overrides.
5. Verify on the matching surface. "Inconclusive" or wrong-surface is not a pass. Flag it.
6. Rebase into small, ordered commits. Stack follow-ups.
   Use the [pstack:principle-sequence-verifiable-units]({{packy.access:pstack:principle-sequence-verifiable-units}}) principle skill, building, verifying, and committing each small unit before the next.
7. If the design is contested, [pstack:interrogate]({{packy.access:pstack:interrogate}}) before shipping.
8. Run **Opening a PR**.

Code-coupled work (one feature, one migration) goes to a single owner with the checkpoint inline. That owner fans out internally after the blocking phase. Parent-level fan-out is for slices that produce independent artifacts (audits, cross-subsystem investigations, competing experiments). Rewrite the checkpoint at phase boundaries. Spawn a fresh owner rather than chaining interrupts.

**Reply:** what you built, what you chose and why, the throughput checkpoint, open decisions. Tables for design alternatives.
