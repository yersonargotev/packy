> For on-demand use, read `../ACCESS.md` first and carry its initiating
> surface/snapshot, target, prepared resource paths and writable output context.
> Resolve playbook paths from the returned resource root.

> Apply the parent skill's host execution contract. Report unavailable
> scheduling, independent review, or live verification as incomplete gates.
> Keep external actions within the user's authorization.

### Investigation

**You own the answer. Plan, route, write.**

Investigation requests are read-only. They produce a cited explanation or a recommendation, not a code change.

1. Route through the [pstack:how]({{packy.access:pstack:how}}) skill. For motivation questions, also route through the [pstack:why]({{packy.access:pstack:why}}) skill.
2. Throughput checkpoint stays one line: `throughput checkpoint: n/a, read-only investigation`.
3. Produce the [pstack:how]({{packy.access:pstack:how}})-shaped output (Overview / Key Concepts / How It Works / Where Things Live / Gotchas), or a recommendation with a tradeoffs table if the request is a decision between alternatives.
4. Apply the [pstack:unslop]({{packy.access:pstack:unslop}}) skill to the reply.

No PR, no babysit, no [pstack:architect]({{packy.access:pstack:architect}}) unless the investigation precedes a code change. If it does, hand back to the user and re-route to Bug fix or Feature.

**Reply:** the investigation output. For "are we sure?" answers, include your real judgment with reasons. Push back if the premise is wrong (see Autonomy).
