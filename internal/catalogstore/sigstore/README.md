# Sigstore trust roots

`tuf-repo.github.com/root.json` is the initial TUF root of GitHub's Sigstore
instance. Packy embeds it to refresh that instance's trusted root from
`https://tuf-repo.github.com`, as the GitHub CLI does.

- Source: https://github.com/cli/cli/blob/aec38089147ae38264668843fa6126022c56a0e4/pkg/cmd/attestation/verification/embed/tuf-repo.github.com/root.json
- Commit: `aec38089147ae38264668843fa6126022c56a0e4` (cli/cli `trunk`, 2026-11-26)
- Git blob: `0d20e8f504d78258ec500d6e00a3876ec17056ad`
- SHA-256: `98cba97be9075bc98b2322de3de85fbd1b70ec7392991dfd2f53d215bede1a8d`

The file is unmodified. cli/cli is distributed under the MIT License:

```text
MIT License

Copyright (c) 2019 GitHub Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
