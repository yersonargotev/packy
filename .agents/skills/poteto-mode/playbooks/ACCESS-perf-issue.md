> For on-demand use, read `../ACCESS.md` first and carry its initiating
> surface/snapshot, target, prepared resource paths and writable output context.
> Resolve playbook paths from the returned resource root.

> Apply the parent skill's host execution contract. Report unavailable
> scheduling, independent review, or live verification as incomplete gates.
> Keep external actions within the user's authorization.

### Perf issue

**You own the measurement story. Plan, review, verify the numbers.** Tie every fix to a measurement, don't read source instead of measuring.

1. Capture a baseline trace via the matching control skill. Vet the baseline, and each later number, with the [pstack:benchmark-checklist]({{packy.access:pstack:benchmark-checklist}}) skill.
2. [pstack:how]({{packy.access:pstack:how}}) to ground hypotheses. Don't claim a perf ceiling without running it first.
   Try the performance mantras in order, cheapest first:
   1. Don't do it. Stop work whose result nothing uses rather than cheapening it.
   2. Do it, but don't do it again.
   3. Do it less.
   4. Do it later.
   5. Do it when they're not looking.
   6. Do it concurrently.
   7. Do it cheaper.

   When an earlier mantra meets the target, stop.
3. Plan the fix from the trace. If it crosses a function boundary, [pstack:architect]({{packy.access:pstack:architect}}) first. Delegate implementation to a subagent using your configured perf-issue model (default `inherit-parent`). Review the diff. Capture a post-fix trace. Match reasoning to task difficulty with supported controls and confirmed role preferences. Use standard speed unless urgency calls for fast mode; verification preserves its configured speed. For `auto` or `inherit-parent`, omit model, effort, and speed overrides.
   Apply the [pstack:principle-sequence-verifiable-units]({{packy.access:pstack:principle-sequence-verifiable-units}}) principle skill, verifying each attempt before trying the next.
4. Parse and compare the artifacts (JSON to sqlite, diff). "Inconclusive" or wrong-surface is not a pass. Flag it.
5. Cite the measurement in the PR.
6. Run **Opening a PR**.

For sustained improvement against a metric rather than a one-off fix, use the Hillclimb playbook (`playbooks/ACCESS-hillclimb.md`).

**Reply:** baseline number, post-fix number, delta, artifact path.
