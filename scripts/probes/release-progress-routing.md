# Release progress routing probe

Run fresh Codex turns with the candidate release instructions and an inert `gh`.
The probe requires a Codex login and access to `gpt-6.1-terra`. It copies the
authentication file to a private temporary directory and never copies it into
the output or checkout. The fixture cannot publish or approve a real deployment.

```sh
python3 -B scripts/probes/release-progress-routing.py \
  --source-root /path/to/candidate-checkout \
  --out-dir /tmp/new-release-routing-evidence
```

Use `--scenario both`, `--scenario refusal`, or `--scenario resume` to select
cases. Inspect `items.json`, `gh.jsonl`, and the report for each case. Inspect the resume case for process termination and wait before replacement.
The report checks command event order and exit codes. Fixture results are not
evidence of a real GitHub release.
