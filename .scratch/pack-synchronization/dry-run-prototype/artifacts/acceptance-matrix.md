# Acceptance matrix

| Area | Case | Expected state | Result |
| --- | --- | --- | --- |
| normalize | unique source | dispatch | pass |
| normalize | ambiguous source | blocked | pass |
| normalize | unknown source | blocked | pass |
| selector | latest-stable | dispatch | pass |
| selector | exact published prerelease | dispatch | pass |
| selector | exact full SHA | dispatch | pass |
| selector | branch, abbreviated SHA, or arbitrary tag | blocked | pass |
| preflight | dirty non-main local checkout | dispatch | pass |
| preflight | invalid remote repo/auth/workflow/config | blocked | pass |
| concurrency | identical active or pending run | attach | pass |
| concurrency | different request behind active run | pending | pass |
| concurrency | new request replaces pending | superseded | pass |
| concurrency | candidate regression | blocked | pass |
| classification | valid AI evidence | continue | pass |
| classification | invalid or below-floor AI evidence | blocked | pass |
| classification | AI unavailable after bounded retries | blocked | pass |
| classification | human first dispatch | awaiting-human-classification | pass |
| classification | human second dispatch | continue | pass |
| classification | multiple affected packs | continue | pass |
| classification | major without migration | blocked | pass |
| classification | stale plan or base | blocked | pass |
| security | malicious upstream scripts/hooks/actions | continue | pass |
| security | forbidden dispatch inputs | blocked | pass |
| publication | no content change | no-op | pass |
| publication | new or updated pristine PR | decision-ready | pass |
| publication | base advanced | blocked | pass |
| publication | edited PR metadata | blocked | pass |
| publication | human commits or divergent branch | blocked | pass |
| publication | closed PR | blocked | pass |
| publication | manual merge | merged | pass |
| recovery | failure before candidate resolution | blocked | pass |
| recovery | failure after candidate resolution | blocked | pass |
| recovery | valid operational artifact | retryable | pass |
| recovery | missing, invalid, or expired artifact | blocked | pass |
| recovery | exact retry | dispatch | pass |
| recovery | success without PR/no-op/human-wait | blocked | pass |
| monitoring | monitoring interrupted | pending | pass |
| monitoring | URL-only mode | request-accepted | pass |
