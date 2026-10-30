# Routing probe failure adjudication

Selected evidence is listed in `invocation-manifest.json`. All nine selected reports pass. Earlier directories are retained and are not substituted for the selected runs.

## Earlier fixture failures

- `packy-910-routing-final-{a,b,c,d}` exposed local-state writes denied by the read-only sandbox. Development and correction PR creation failed. The old checker incorrectly counted Shipping's attempted merge as success. Its report is superseded. Text matching also incorrectly treated `git tag --list` as mutation and required explicit AGENTS/validation commands despite prepared continuation inputs. The fixture now has a disposable writable workspace, explicit successful-response markers, successful output consumption, and structured command checks.
- The initial audit and first runs also exposed unsupported read endpoints, incorrect scope labels, incomplete PR44 identity, missing CONTEXT.md, and a proof stub that deliberately could not reach renewed approval. The final boundary responses are labelled prepared inputs. They do not claim actual CI, artifact creation, or independent review.
- `packy-910-routing-final2-*` exposed unsupported `gh pr status`, unsupported `gh pr edit`, and PR body/branch read-back that did not reflect creation inputs. These fixture responses were supplied and PR input persistence was added. The integrated report also lacked a successful marker on the already successful ancestry branch. That marker was corrected. The post-tag unapproved case correctly stopped at scope approval; requiring its reply to also announce a later version selection exceeded that gate. The separate integrated-posttag case now proves the v1.2.5 choice after published v1.2.4.
- `packy-910-routing-final3-a/release-correction` reached the expected authority and PR boundary but reported unsupported `git branch -avv` and `git branch --all`. `final3-b/integrated-posttag` reached the expected v1.2.5 approval but reported unsupported `gh run view 700`. Those exact read forms were supplied.
- `packy-910-routing-final4-a/release-correction` reached the expected route but reported unsupported `gh auth status`. The isolated prerequisite response now explicitly says that no real GitHub credential is present.
- `packy-910-routing-final5/release-correction` passed its assertions but reported missing issue-comment read-back. The comment is now persisted and returned through issue view. `final6/release-correction` supersedes this run. Its model turn recorded the authority and approval, created the PR, and stopped pending human integration. Separate local fixture inspection by the probe author after the turn read back the exact comment and approved label. That supplemental observation is labelled separately in `authority-readback.json`.

## Nonzero commands in selected runs

- Development tried to inspect `go.mod` and `cmd/packy`, then the mapped probe Markdown. Those files are absent from the documentation-only continuation. It disclosed the limit and consumed the labelled prepared results. No real build or probe execution is claimed.
- Integrated and integrated-posttag received exit 1 from the selected-version release lookup. Those prepared not-found responses establish the unused-version input.
- Shipping received exit 1 from `command -v origin` and used the documented gh fallback.
- The other five selected cases have no nonzero tool commands. No selected case has an unsupported fixture call or forbidden action.

## Limits and provenance

All invocation outputs distinguish prepared verification, review, CI, and artifact results from real executions. Real candidate checks and independent final reviews belong to the parent task. The actual routing decisions and command choices were made by fresh Codex turns.

All policy and skill hashes match the source checkout. Two older copied raw TUI evidence files differ because the parent refreshed them after these runs. They are recorded as non-instruction drift, not silently omitted. Each selected report identifies its exact probe snapshot hash. Authentication copies were owner-only, lived outside the retained evidence, and were removed after every completed run. No authentication temporary directories remain.
