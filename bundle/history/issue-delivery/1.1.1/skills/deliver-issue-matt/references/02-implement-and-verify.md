# Implement and verify

Read and execute the complete vendored workflow at
[`implement.md`](implement.md) against the ready issue. Treat the qualified
starting commit as `/code-review`'s fixed point and the complete fetched issue
as its Spec source.

A seam is pre-agreed when the issue or its agent brief identifies an observable
public interface. Before writing tests against any other seam, propose it and
pause for confirmation. When TDD is impractical, record the concrete reason and
prove behavior at the nearest public boundary.

Adjudicate every Standards and Spec finding. For accepted findings, run one
repair cycle; the vendored workflow's closing `/code-review` reruns both axes
in independent contexts. Review passes only when both axes pass on the exact
unchanged candidate.

After review, derive manual verification from the actual changed surface:

- For a CLI, run the real command with isolated state and check output, exit
  status, and effects.
- For a web interface, use Browser, Chrome, or another available interactive
  tool to exercise and inspect the visible behavior.
- For an API, call its public endpoint in an authorized local or test
  environment.
- For a library, exercise a minimal consumer through its public interface.

Use local, sandbox, or explicitly authorized test environments. Mark manual
verification `Not applicable` only when no practical user-observable path
exists, and record the concrete reason. Check the complete expected final
state, including required absences and cleanup of verification state.

A manual-verification finding starts one repair cycle. The vendored workflow
establishes full-suite and review proof for the new candidate; then every
affected manual scenario runs again. Before opening a change request, require a
clean worktree and bind the full suite, Standards, Spec, and manual-verification
results to the exact candidate SHA.

**Complete when:** the clean exact candidate implements every criterion,
passes the complete automated suite and both independent review axes, and
passes every applicable manual scenario or carries a concrete non-applicability
reason.
