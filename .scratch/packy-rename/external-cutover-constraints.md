# Packy external cutover constraints

Date checked: 2026-07-17 (America/Bogota)
Question: [GitHub issue #51](https://github.com/yersonargotev/matty/issues/51)
Repository snapshot: [`4ce9a3d`](https://github.com/yersonargotev/matty/tree/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a)

## Conclusion

An in-place rename from `yersonargotev/matty` to `yersonargotev/packy` is technically feasible. The repository rename preserves the repository and its normal web/Git history through redirects, but it does **not** make `github.com/yersonargotev/packy` the same Go module as `github.com/yersonargotev/matty`, and GitHub Actions deliberately does not follow redirects for actions or reusable workflows. The safe cutover therefore needs one coordinated identity commit, the repository rename, a new (not reused) release tag, and a matching Homebrew update.

The recommended release boundary is the first unused version after the current `v0.1.6` (currently `v0.1.7`): existing `v0.1.0`–`v0.1.6` tags/releases and their `matty_*` assets remain historical Matty artifacts, while the first Packy tag is created only from a commit whose module, binary, release workflow, formula generator, schema identifiers, and documentation all say Packy. The already chosen clean incompatible cutover also means Homebrew must not add rename metadata: the old formula is removed and the known installation is explicitly uninstalled/reinstalled as Packy.

## Current namespace and publication facts

These are observations, not reservations; recheck them immediately before the cutover.

| Surface | Checked fact | Primary evidence |
| --- | --- | --- |
| GitHub repository | The authenticated GitHub API returned `404` for `yersonargotev/packy`; no repository with that owner/name was visible. The current repository is public `yersonargotev/matty`, default branch `main`. | [`GET /repos/yersonargotev/packy`](https://api.github.com/repos/yersonargotev/packy), [`GET /repos/yersonargotev/matty`](https://api.github.com/repos/yersonargotev/matty) |
| Go module | `go.mod` declares `github.com/yersonargotev/matty`. The public Go proxy lists `v0.1.0` through `v0.1.6` for that path and returned `404` for the Packy path. | [`go.mod`](https://github.com/yersonargotev/matty/blob/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/go.mod#L1), [Matty proxy versions](https://proxy.golang.org/github.com/yersonargotev/matty/@v/list), [Packy proxy versions](https://proxy.golang.org/github.com/yersonargotev/packy/@v/list) |
| Tags and releases | The repository has releases/tags `v0.1.0`–`v0.1.6`; `v0.1.6` is latest and is not immutable. | [Latest release API](https://api.github.com/repos/yersonargotev/matty/releases/latest), [tags API](https://api.github.com/repos/yersonargotev/matty/tags) |
| Release assets | `v0.1.6` has `checksums.txt` plus four `matty_v0.1.6_{darwin,linux}_{amd64,arm64}` binaries. The build script intentionally produces the `matty_...` names. | [`v0.1.6` release](https://github.com/yersonargotev/matty/releases/tag/v0.1.6), [`build-release-artifacts.sh`](https://github.com/yersonargotev/matty/blob/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/scripts/build-release-artifacts.sh#L61-L70) |
| GitHub Actions | This repository contains no `action.yml`/`action.yaml`, no `workflow_call`, and no self-reference of the form `uses: yersonargotev/matty...`. A public GitHub code search found no indexed external references containing the exact old repository string. Its workflows consume third-party actions and contain operational scripts/docs with hard-coded `yersonargotev/matty`. | [workflow directory](https://github.com/yersonargotev/matty/tree/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/.github/workflows), [repo code search](https://github.com/search?q=%22yersonargotev%2Fmatty%22&type=code) |
| Schemas | Five schema files use `$id` values under `https://github.com/yersonargotev/matty/workflows/schemas/...`; every one currently returns `404` because it is neither a GitHub `blob/...` URL nor a raw-content URL. | [`workflows/schemas`](https://github.com/yersonargotev/matty/tree/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/workflows/schemas), [example current `$id`](https://github.com/yersonargotev/matty/workflows/schemas/pack-source-dispatch.schema.json), [working raw form](https://raw.githubusercontent.com/yersonargotev/matty/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/workflows/schemas/pack-source-dispatch.schema.json) |
| Homebrew | `yersonargotev/homebrew-tap` contains `Formula/matty.rb` at `v0.1.6`; it installs a binary as `matty`. It has no `Formula/packy.rb` and no `formula_renames.json`. Homebrew's public formula API returned `404` for a core `packy` formula. | [current tap formula](https://github.com/yersonargotev/homebrew-tap/blob/7603485b5071db932e83f6edf9f83b69960cd0f3/Formula/matty.rb), [tap Packy path](https://github.com/yersonargotev/homebrew-tap/blob/main/Formula/packy.rb), [Homebrew formula API](https://formulae.brew.sh/api/formula/packy.json) |
| Documentation hosting | The repository has no configured homepage and the authenticated Pages API returned `404`, so no current GitHub Pages site was found. Repository documentation is Markdown, mostly using relative links. | [repository API](https://api.github.com/repos/yersonargotev/matty), [Pages API](https://api.github.com/repos/yersonargotev/matty/pages), [`README.md`](https://github.com/yersonargotev/matty/blob/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/README.md) |

## Constraints by surface

### 1. In-place GitHub repository rename

- Repository admin permission is required. GitHub redirects normal web traffic and old clone/fetch/push URLs to the renamed repository, but recommends changing every clone's `origin` to the new URL. Issues, stars, followers, and wiki content stay with the repository. [GitHub: Renaming a repository](https://docs.github.com/en/repositories/creating-and-managing-repositories/renaming-a-repository)
- Redirects are conditional future compatibility, not an alias to own forever: creating a new repository at the old `yersonargotev/matty` name disables the redirect. Therefore the old repository name must not be reused after cutover. [GitHub: Renaming a repository](https://docs.github.com/en/repositories/creating-and-managing-repositories/renaming-a-repository)
- GitHub Pages URLs are the documented exception to automatic repository-rename redirects. No Pages site is currently configured, but this must be rechecked at execution time. [GitHub: Renaming a repository](https://docs.github.com/en/repositories/creating-and-managing-repositories/renaming-a-repository)
- The `packy` name is only observed free in this owner's repository namespace. A `404` is not a reservation and does not establish package, domain, or trademark availability.

### 2. Go module identity and tag lineage

- The `module` directive is the module's unique identifier and the prefix for every package import. Changing it from `github.com/yersonargotev/matty` to `github.com/yersonargotev/packy` creates a distinct module identity even though GitHub redirects the repository. Consumers must change imports; a GitHub rename alone does not do that. [Go `go.mod` reference](https://go.dev/doc/modules/gomod-ref), [Go Modules wiki](https://go.dev/wiki/Modules)
- Existing tags cannot be reinterpreted as Packy versions: the commits at `v0.1.0`–`v0.1.6` declare the Matty module path. Go tools enforce consistency between the requested import path and the path declared by that version's `go.mod`. [Go Modules wiki](https://go.dev/wiki/Modules), [Go module path reference](https://go.dev/ref/mod#module-paths)
- Keep the repository's semantic-version sequence, but publish Packy only at a fresh tag created after the module-path change. Go maps a root module version directly to the same Git tag name, and versions are immutable snapshots. [Go Modules reference: mapping versions to commits](https://go.dev/ref/mod#mapping-versions-to-commits)
- Do not move or delete the old tags. Go warns that changing a tag can produce authentication errors and that deleted tag content may remain on module proxies; the public proxy already serves all seven Matty versions. [Go Modules reference](https://go.dev/ref/mod#module-proxy), [Go module mirror](https://go.dev/blog/module-mirror-launch)

### 3. GitHub Actions and automation references

- GitHub Actions does not follow repository redirects for actions **or reusable workflows**. Any external `uses: yersonargotev/matty...` caller would fail after rename and must be changed to Packy. [GitHub: Reusing workflow configurations](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations#access-to-reusable-workflows)
- The current repository is not an action/reusable-workflow publisher, and no exact external caller was found in public indexed code, so this is a low observed risk rather than proof that private/unindexed callers do not exist.
- Normal workflows stored in this repository stay with the in-place repository. However, repository literals in release automation and the repo-local synchronization skill must change; dynamic GitHub contexts should be preferred where the repository identity is intended to follow the current repository. GitHub defines contexts as the supported way for workflows to access run/repository information. [GitHub Actions contexts](https://docs.github.com/en/actions/concepts/workflows-and-actions/contexts)
- Do not publish a release or dispatch the pack-source workflow during the transition. The current release workflow writes the old formula and old asset URLs, while synchronization scripts explicitly address the old repository. [release workflow](https://github.com/yersonargotev/matty/blob/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/.github/workflows/release.yml), [sync skill requests](https://github.com/yersonargotev/matty/blob/4ce9a3dbae4d754fdba26c90c0cfe85a4cdda86a/.agents/skills/sync-pack-source/REQUESTS.md)

### 4. Schema identifiers

- JSON Schema `$id` is a schema URI identifier/base URI. It need not be network-addressable, but it should be a stable absolute URI controlled by the project; consumers may use it as a registry key or resolve `$ref` values against it. [JSON Schema: schema identification and `$id`](https://json-schema.org/understanding-json-schema/structuring#id)
- All five current `$id` values embed the old product/repository identity and are already non-dereferenceable. The cutover should replace them intentionally rather than depend on GitHub redirects. If network retrieval is a requirement, use a tested content URL or a project-controlled stable schema domain; a GitHub `raw` URL pinned to a commit is immutable but cannot be the evergreen identity of an evolving schema.
- Treat changing `$id` as a schema-identity change. Update every producer, validator registry, fixture, example, and documentation reference in the same identity commit. The repository currently has no cross-schema `$ref`, which reduces the internal migration risk.

### 5. Homebrew formula and tap

- The tap repository itself does not need renaming: GitHub-hosted taps conventionally use a `homebrew-` repository prefix, and `yersonargotev/homebrew-tap` already provides the short tap name `yersonargotev/tap`. [Homebrew: How to create and maintain a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
- Homebrew's official compatibility-oriented formula-rename procedure is atomic in the tap: rename the file and Ruby class, delete the old formula, add `{"matty":"packy"}` to `formula_renames.json`, and make the new formula pass strict audit. [Homebrew: Renaming a formula](https://docs.brew.sh/Rename-A-Formula)
- That official migration path is intentionally rejected here because `formula_renames.json` migrates an installed old formula to its new name, contradicting the fixed clean/incompatible cutover. The tap must instead delete `Formula/matty.rb`, add `Formula/packy.rb`, omit `formula_renames.json`, and require the sole known installer to explicitly uninstall `matty` and install `yersonargotev/tap/packy`. This is deliberately not Homebrew's documented compatibility rename workflow.
- The Packy formula must not land with URLs for assets that do not exist. The new formula's class, asset basename matching, installed binary name, test command, homepage, version, URLs, and SHA-256 values must all change together after the Packy release assets/checksums are available.
- A future formula named `packy` in `homebrew/core` would win an unqualified `brew install packy`; the fully qualified `brew install yersonargotev/tap/packy` remains unambiguous. No current core collision was observed. [Homebrew taps: duplicate formula names](https://docs.brew.sh/Taps#formula-with-duplicate-names)

### 6. Releases, artifacts, and documentation links

- GitHub releases are based on Git tags; attached assets have independent filenames. GitHub permits an authorized user to rename a mutable release asset, but rewriting the existing `matty_*` assets would change historical download URLs and is unnecessary for an identity-only cutover. Preserve them and start `packy_*` names at the first Packy release. [GitHub: About releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases), [REST release asset update](https://docs.github.com/en/rest/releases/assets#update-a-release-asset)
- Future formula URLs and user documentation must use the exact new asset filename because direct release downloads include the asset name. GitHub documents the stable `/releases/latest/download/ASSET` form for latest-release assets, but version-pinned Homebrew URLs should continue to include the exact tag and checksummed artifact. [GitHub: Linking to releases](https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases)
- Relative Markdown links inside the same repository require no host/repository rewrite. Absolute `github.com/yersonargotev/matty/...` links normally redirect after the rename, but all current-product links should still be rewritten to Packy so correctness does not depend on retaining the old-name redirect. Preserve old URLs only where they identify authentic Matty history.
- Raw-content URLs and schema identifiers should not be assumed to redirect; replace and test every current-product raw/schema URL explicitly. GitHub separately documents that raw file URLs do not follow branch renames, illustrating why raw endpoints should not be treated like normal repository web links. [GitHub: Renaming a branch](https://docs.github.com/en/repositories/configuring-and-managing-repositories/managing-branches-in-your-repository/renaming-a-branch)

### 7. What “Packy is available” does and does not mean

- At the check time, the target repository, new Go proxy path, own-tap formula, and Homebrew core formula were all absent. Those technical names are therefore available enough to attempt the coordinated cutover, subject to a last-second recheck.
- This research did **not** establish legal/trademark clearance, domain availability, or uniqueness across other package ecosystems. USPTO says a comprehensive clearance search is broader than a federal database lookup, and WIPO recommends checking national/regional registers and notes that even its database results are not a legal opinion. [USPTO: Federal trademark searching](https://www.uspto.gov/trademarks/search/federal-trademark-searching), [WIPO Global Brand Database](https://www.wipo.int/en/web/global-brand-database)

## Order-sensitive and irreversible operations

1. **Freeze publication/synchronization.** Do not dispatch release or source-sync workflows until the identity commit is live and their repository literals are verified.
2. **Recheck technical names.** Confirm the Packy repository still returns `404`, the own tap still lacks `Formula/packy.rb`, and no core formula now claims the short name.
3. **Prepare and validate one identity commit before mutation.** It should change the Go module/imports, command/binary, release asset generator, formula generator, schema IDs, workflow/skill repository literals, and current-product documentation. Do not tag it yet.
4. **Rename the GitHub repository in place.** Immediately update the local `origin`; never create a new `yersonargotev/matty` repository afterward, because doing so permanently defeats the redirect.
5. **Land the validated identity commit in Packy and let required CI finish.** The short interval where the renamed repository still contains Matty code is safer than merging code that points at a not-yet-existing Packy repository. Keep release/sync workflows frozen during that interval.
6. **Create a fresh next tag only after the Packy commit is on `main`.** Never move/reuse an existing tag. Build `packy_*` assets and `checksums.txt`, verify them, then publish the release.
7. **Update Homebrew only against the published Packy assets.** Delete `Formula/matty.rb`, add `Formula/packy.rb`, do not add `formula_renames.json`, explicitly uninstall the sole known Matty installation, and verify a sandboxed `brew install yersonargotev/tap/packy` without touching the operator's real Homebrew/user configuration during validation.
8. **Verify externally.** Check the new repository/issue/release links, clone/fetch using the new remote, `go list -m github.com/yersonargotev/packy@<new-tag>` with disposable caches, schema retrieval/registry identity, Actions runs, release downloads/checksums, and all current install/docs links.

The operations with durable consequences are: publishing the first Packy Go version (module versions are immutable), creating the new release/tag, changing schema `$id` values consumed externally, deleting the old Homebrew formula without migration metadata, and reusing the old GitHub repository name (which destroys redirects). The repository rename itself can be renamed again, but external references created during either name will not all share the same redirect guarantees.

## Open gaps

1. **Schema URI policy:** choose whether `$id` is a stable logical identifier or a dereferenceable hosted resource. The current values accidentally achieve neither.
2. **Legal name clearance:** perform a human trademark/domain clearance appropriate to the intended markets if Packy will be promoted beyond this personal/open-source cutover. WIPO's terms prohibit automated querying, so it was intentionally not scraped here. [WIPO Global Brand Database terms](https://www.wipo.int/en/web/global-brand-database/terms_and_conditions)
3. **Private/unindexed Actions consumers:** public code search found none, but only repository/dependency owners can confirm private callers.

## Checks performed

- Read issue #51 and the repository/tap state using `gh` API/CLI read operations.
- Inspected module, workflow, schema, release-builder, Homebrew-generator, and documentation identity strings with targeted `rg`/file reads.
- Queried GitHub repository, releases, tags, Pages, and public code-search endpoints without mutation.
- Queried `proxy.golang.org` module lists, Homebrew's formula API, own-tap raw paths, and every current schema `$id` with HTTP GET/HEAD-equivalent reads.
- Compared all external claims with first-party GitHub, Go, Homebrew, JSON Schema, USPTO, and WIPO documentation.
