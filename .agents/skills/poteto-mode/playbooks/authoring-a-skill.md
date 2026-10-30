> Apply the parent skill's host execution contract. Report unavailable
> scheduling, independent review, or live verification as incomplete gates.
> Keep external actions within the user's authorization.

### Authoring or modifying a skill

**You own the skill's voice.**

1. Use the **skill-authoring guide** available on this host (or write and validate the skill directly).
2. Validate the skill: frontmatter has `name` and `description`, referenced files exist, cross-skill links resolve.
3. Test cases if structural. Skip if subjective.
4. Run **Opening a PR**.

When in doubt, delete. Keep only prose that changes a decision. Tell it to do the thing and skip the reason. Explain only when the rule is confusing without one. Match tone to scope. Point at structural sources (types, READMEs, config) per the **encode-lessons-in-structure** principle skill. Delegate to other skills by path. Don't restate. A workflow you keep hitting but isn't captured → propose a new skill.

**Reply:** summary of the skill, key design decisions, validation notes.
