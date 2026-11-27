# Sigstore test fixtures

These fixtures let tests verify a GitHub Sigstore instance attestation offline.

`attest-demo.sigstore.json` is the build-provenance bundle of
`github_provenance_demo-0.0.12-py3-none-any.whl` (SHA-256
`ae57936def59bc4c75edd3a837d89bcefc6d3a5e31d55a6fa7a71624f92c3c3b`), signed by GitHub's Sigstore instance for
`actions/attest-demo/.github/workflows/build-python.yml@refs/heads/main` on a
GitHub-hosted runner.

- Source: https://github.com/cli/cli/blob/aec38089147ae38264668843fa6126022c56a0e4/pkg/cmd/attestation/test/data/github_provenance_demo-0.0.12-py3-none-any-bundle.jsonl
- Commit: `aec38089147ae38264668843fa6126022c56a0e4`
- SHA-256: `4f8c096e38a0eee242574ab100d16701928605409225e59784a3636f742bb27e`
- License: MIT, as recorded in [../../sigstore/README.md](../../sigstore/README.md).

`github-trusted_root.json` is the `trusted_root.json` target of
`https://tuf-repo.github.com`, fetched through TUF from the embedded
[root](../../sigstore/tuf-repo.github.com/root.json) on 2026-11-27.

- SHA-256: `484cdfe1a7c65479c5ba2a22193d1be90f0020db1997de696ab207434c62fbb7`

Both files are unmodified.
