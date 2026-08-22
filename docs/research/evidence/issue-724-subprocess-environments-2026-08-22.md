# Issue 724: isolated environments for test-owned subprocesses

Date: 2026-08-22

Repository base: [`bce18b693b1d2c789e263406e4ab37641bc8f321`](https://github.com/yersonargotev/packy/commit/bce18b693b1d2c789e263406e4ab37641bc8f321)

Scope: [issue #724](https://github.com/yersonargotev/packy/issues/724), the cleanup repair in [PR #719](https://github.com/yersonargotev/packy/pull/719), current test and production process boundaries, official Git documentation, official Go documentation and source, and the XDG Base Directory specification. The research phase changed no code, issue, or label; its decisions were subsequently incorporated into #724 and the issue was moved to `status:approved` on the same date.

## Conclusion

The proposed `internal/testprocess` package is a useful deep test module. Issue #724 adopted the decisions below and is now sufficiently specified for `status:approved`. The module should own one narrow concern: constructing an explicit, sorted child environment and creating its disposable user, temporary, and XDG directories. It must not own `exec.Command`, context, working directory, I/O, network confinement, arguments, or expected output.

The issue body reviewed at the start of this research was not approval-ready verbatim because “Every test that launches a real child process” crossed production-owned process boundaries. Tests reach real Git through `internal/bootstrap`, production Git and Go through Managed Pack Promotion, the real offline-validation worker, and Claude Smoke's product-specific sandbox. Replacing those environments with a test helper would either change production behavior or stop testing the production contract. The approved implementable seam is instead:

> Every child whose environment is owned at a test call site—because test code or a test-only helper constructs the `exec.Cmd`, or because the test receives a bare `exec.Cmd` and completes it before launch—uses the shared test environment. A test may use a smaller explicit environment when the absence of a variable is the behavior under test. A test that runs a production-owned process boundary preserves and verifies that boundary's production environment.

On the fixed base there are 17 `exec.Command`/`exec.CommandContext` construction sites in `*_test.go`. Fifteen belong to the shared-helper migration, one deliberately launches the current test executable with no `HOME` or `PATH`, and one deliberately supplies `isolatedGitEnvironment` to a real Git process to test a production environment. Three additional Claude Smoke launch sites receive a bare command from the production sandbox factory with `Env == nil`; the tests should set the shared environment before launching those commands without changing the factory.

## Method

The census searched every Go test for direct `exec.Command` construction and every production `exec.Command` site for paths exercised by tests. Each apparent command was traced to its `Run`, `Output`, or `CombinedOutput` call; fake runners and command values that are never started were excluded. The current issue, PR #719, its final evidence, the accepted Managed Pack ADR, and the exact repair commits were read through GitHub. Environment semantics were checked against the owning projects' documentation and source.

An execution check also ran `go run ./internal/tools/packdocs --check --root .` with fresh writable `GOCACHE`, `GOMODCACHE`, and `GOPATH`, an empty user/XDG root, `GOTOOLCHAIN=local`, `GOVCS=*:off`, `GOSUMDB=off`, and every Go dependency network path disabled. `GOPROXY=off` cannot fill a fresh module cache; `GOPROXY=file://<outer-GOMODCACHE>/cache/download` succeeded by copying already provisioned module artifacts from the local Go proxy cache. This is the same dependency-source pattern already used by the production promotion gate ([gate environment](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/gates.go#L71-L107)).

## Census

### Test-owned launches to migrate

| Test-owned subprocess | Current environment | Classification and required change |
| --- | --- | --- |
| Repository-candidate fixture Git in `gitOutput`, `gitOutputNoTest`, and `runTestCommandEnv` ([constructors and local builder](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/preparer_test.go#L574-L631)) | A sorted allowlist, but one XDG path is not created, one helper uses a shared fixed `/tmp` path, and it omits Git maintenance and Go-descendant protections. | In scope. Replace `testGitEnvironment`; keep author/committer variables as explicit additions. |
| Authority-phase fixture and inspection Git in `runTestCommand` ([helper](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/authorityphase/adapter_test.go#L227-L250)) | `Env == nil`, so it inherits every host variable. | In scope. This missing package is the clearest gap in the current implementation plan. |
| Claude Smoke fixture Git, logged stub, and interposer processes ([first Git fixture](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner_test.go#L232-L272), [logged stub](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner_test.go#L460-L486), [interposer launches](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner_test.go#L600-L646), [second Git fixture](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner_test.go#L682-L720)) | The Git fixtures have a bespoke allowlist but no maintenance/telemetry invariant; the stub and interposer commands inherit the host. | In scope. Compose the shared base with fixture-specific `PATH` and author identity. Do not replace `RestrictedEnv` or `acquisitionEnv`. |
| Claude Smoke write/read-boundary commands returned by `sandboxCommand` and launched in tests ([launch sites](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner_test.go#L169-L211)) | The factory returns a bare command and these tests launch it without assigning `Env`, so Go supplies the host environment. | In scope at the test call sites: assign the shared environment after receiving the command. The sandbox factory and its production policy remain unchanged. |
| Root-Go boundary `go list` and `go test` ([helper](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/ci/inert_bundle_go_boundary_test.go#L54-L75)) | A bespoke offline allowlist with fresh cache paths, but its `HOME`/XDG/cache directories are not created and it lacks telemetry protection. | In scope. Use the Go-offline variant. This fixture has no external module dependencies, but using the same variant keeps the contract uniform. |
| Pack-documentation `go run` checks ([both constructors](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/ci/pack_documentation_test.go#L11-L52)) | `Env == nil`; it inherits credentials, Go configuration, writable caches, telemetry mode, proxies, and network-capable defaults. | In scope. Use the Go-offline variant with fresh writable caches and the outer module download cache as a local `file://` proxy. |
| Pack Source fixture Git ([constructor](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/packsync/engram_source_test.go#L51-L62)) | `Env == nil`. | In scope. Use the base environment and explicit author configuration already issued through Git commands. |
| `syncpacksource` fixture Git ([builder and constructor](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/tools/syncpacksource/single_source_admission_test.go#L376-L406)) | Copies almost all of `os.Environ` and filters only four keys. Host credentials, proxy, locale, Git runtime configuration, and Go telemetry policy remain observable. | In scope. Delete the partial builder and use the shared base. |
| CLI source-fixture Git ([helper](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/cli/root_test.go#L944-L978)) | Appends temporary `HOME`, XDG config, and `GIT_CONFIG_NOSYSTEM` to the complete host environment. The XDG path is not created and Git maintenance/global config remain incomplete. | In scope for the direct fixture helper. The production Git called by `init` remains out of scope. |

Go's `os/exec` contract explains the observed ambient inheritance: `Cmd.Env == nil` uses the current process environment, and duplicate keys use the last value ([official `os/exec.Cmd` documentation](https://pkg.go.dev/os/exec#Cmd)). A deterministic allowlist avoids both ambient inheritance and the fragile “append a duplicate override” pattern.

### Intentional minimal environment

`TestProjectVerifyRunsWithNoHomeOrPath` re-executes the absolute current test executable with exactly two `PACKY_VERIFY_*` variables ([test and helper](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/cli/project_verify_test.go#L36-L73)). The absence of `HOME` and `PATH` is the subject of the test, not an isolation defect. It must remain an explicit minimal environment and the acceptance criteria must permit it. Adding even the shared temporary `HOME` would invalidate the proof.

The general rule should be: every launched child has a non-nil, explicit environment; the shared base is required unless a named test asserts that one or more base variables are absent. Exceptions must be local and explanatory, not a second generic builder.

### Production-owned subprocesses reached by tests

These real children are part of the complete census but must not import or be replaced by `internal/testprocess`:

| Production owner reached during tests | Evidence | Why it stays production-owned |
| --- | --- | --- |
| Bootstrap Installed Source Git, reached by `internal/bootstrap` tests and CLI/TUI initialization tests | `runGit` assigns `gitEnv`, which currently derives from `os.Environ` ([production boundary](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/bootstrap/bootstrap.go#L392-L418)); bootstrap tests select stub Git through `t.Setenv` ([representative tests](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/bootstrap/installed_source_test.go#L94-L153)); the TUI also exercises a real clone ([TUI initialization test](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/cli/tui_backend_test.go#L205-L245)). | The production function constructs and owns the child environment. Removing all global mutation here requires a production dependency-injection decision, not a test environment helper. The current issue must not claim repository-wide removal of `t.Setenv`. |
| Repository-candidate production Git | `run` always applies `isolatedGitEnvironment`, including runtime Git config for maintenance and GC ([production implementation](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/preparer.go#L586-L633)). | Tests must exercise the production Git policy. The direct real-Git probe that passes `isolatedGitEnvironment` is therefore not migrated ([contract test](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/preparer_test.go#L399-L417)). |
| Repository-candidate production gates and their Go cache resolver | `runSanitized` owns a credential-free offline environment and `currentGoCaches` runs the absolute Go tool ([production gates](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/gates.go#L17-L147)); tests run the suite gate and use it to build the private worker ([gate test](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/gates_test.go#L10-L21), [integration build](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/repositorycandidate/integration_test.go#L896-L902)). | This is admission-gate production policy, including its own offline module source. It is explicitly outside the proposed test module. |
| Authority-phase Go resolver, sanitized Git snapshot, and worker invocation | The adapter builds the production prepublication environment, resolves Go through the absolute `runtime.GOROOT`, snapshots via Git, and passes the result to its runner ([adapter flow](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/authorityphase/adapter.go#L47-L87), [environment and resolver](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/authorityphase/adapter.go#L207-L275), [Git command](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/authorityphase/adapter.go#L309-L380)). | ADR 0038 makes this a distinct authority boundary. Tests that reach it must preserve that environment; only their separate fixture Git helper is migrated. |
| Offline-validation production worker | The live test calls `New(executable).Validate`, which starts the network-denied production worker ([live test](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/offlinevalidation/adapter_test.go#L25-L42)); the adapter creates and validates its exact minimal environment ([production environment](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/offlinevalidation/adapter.go#L46-L99), [environment builder](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/managedpackpromotion/offlinevalidation/adapter.go#L131-L163)). | The environment and platform sandbox are the behavior under test. A generic test base would weaken the proof and change a production policy. |
| Claude Smoke product subprocesses | `RestrictedEnv`/`acquisitionEnv` define product-specific writable roots and traffic policy, and the runner assigns those environments to sandboxed commands ([product allowlists](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner.go#L693-L727), [sandboxed launch](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/internal/claudesmoke/runner.go#L627-L662)). | Preserve these allowlists and their fake environment tests. Only test-owned fixture/stub/interposer environments and bare commands completed by tests use `testprocess`. |

Fake `Runner` implementations, environment-model tests that never start a process, and tests that only inspect a constructed `exec.Cmd` remain unchanged.

## Exact base environment

The base is an allowlist, not filtered `os.Environ`. Its only ambient value is the host `PATH`; if `PATH` is unavailable, construction should fail rather than silently invent a different tool-resolution policy. Command name/path selection remains visible at each call site. In particular, setting `cmd.Env` does not change the executable that `exec.Command("git")` already resolved through the parent process's `PATH`; a test stub should be invoked by its explicit path and may override child `PATH` only when descendants must find it.

Each call creates one unique `t.TempDir` root and real mode-`0700` directories for:

- `HOME=<root>/home`
- `TMPDIR=<root>/tmp`
- `XDG_CACHE_HOME=<root>/xdg/cache`
- `XDG_CONFIG_HOME=<root>/xdg/config`
- `XDG_DATA_HOME=<root>/xdg/data`
- `XDG_STATE_HOME=<root>/xdg/state`

These are the persistent user-directory classes defined by the [XDG Base Directory specification](https://specifications.freedesktop.org/basedir/0.8/). `XDG_RUNTIME_DIR` should be omitted: none of the censused children needs user-session sockets or named pipes, and the XDG specification gives that directory stronger ownership, local-filesystem, and login-lifetime semantics than an ordinary test data root.

The complete base map is:

```text
GIT_CONFIG_COUNT=2
GIT_CONFIG_GLOBAL=<os.DevNull>
GIT_CONFIG_KEY_0=maintenance.auto
GIT_CONFIG_KEY_1=gc.auto
GIT_CONFIG_NOSYSTEM=1
GIT_CONFIG_VALUE_0=false
GIT_CONFIG_VALUE_1=0
GIT_TERMINAL_PROMPT=0
GO_TELEMETRY_CHILD=2
HOME=<root>/home
LANG=C
LC_ALL=C
PATH=<host PATH>
TMPDIR=<root>/tmp
XDG_CACHE_HOME=<root>/xdg/cache
XDG_CONFIG_HOME=<root>/xdg/config
XDG_DATA_HOME=<root>/xdg/data
XDG_STATE_HOME=<root>/xdg/state
```

Return entries in lexicographic key order. Use `os.DevNull` in code; it is `/dev/null` on the current Unix platforms without hard-coding a platform path.

`GIT_CONFIG_GLOBAL` prevents reads of both `$HOME/.gitconfig` and `$XDG_CONFIG_HOME/git/config`; `GIT_CONFIG_NOSYSTEM` skips system configuration; and `GIT_TERMINAL_PROMPT=0` disables terminal credential prompting ([official Git environment documentation](https://git-scm.com/docs/git#Documentation/git.txt-codeGITCONFIGGLOBALcode-codeGITCONFIGSYSTEMcode)). Runtime configuration is expressed entirely through the environment:

- `GIT_CONFIG_COUNT=2`
- `GIT_CONFIG_KEY_0=maintenance.auto`, `GIT_CONFIG_VALUE_0=false`
- `GIT_CONFIG_KEY_1=gc.auto`, `GIT_CONFIG_VALUE_1=0`

Git documents that these indexed pairs become command-scope configuration, override configuration files, and are themselves overridden only by explicit `git -c` arguments ([official `git-config` environment contract](https://git-scm.com/docs/git-config#Documentation/git-config.txt-codeGITCONFIGCOUNTcode-codeGITCONFIGKEYltngtcode-codeGITCONFIGVALUEltngtcode)). `maintenance.auto=false` stops commands from launching `git maintenance run --auto`; `gc.auto=0` disables the older automatic-GC threshold ([official maintenance configuration](https://git-scm.com/docs/git-maintenance#Documentation/git-maintenance.txt-maintenanceauto), [official GC configuration](https://git-scm.com/docs/git-gc#Documentation/git-gc.txt-gcauto)). Both are needed because PR #719 observed an automatic Git writer after the parent command had exited.

`GO_TELEMETRY_CHILD=2` is a descendant-lifecycle guard, not a general Go configuration replacement. The Go telemetry implementation treats value `2` as “executed directly or indirectly by a child” and returns without starting another telemetry sidecar ([official Go telemetry source](https://github.com/golang/telemetry/blob/06ef541f3fa34829fc96f4773727f9c35e117768/start.go#L77-L101), [constant semantics](https://github.com/golang/telemetry/blob/06ef541f3fa34829fc96f4773727f9c35e117768/start.go#L131-L142)). This matters even with isolated XDG config because Go telemetry normally stores counters below `os.UserConfigDir()/go/telemetry` and local mode writes memory-mapped counter files ([official Go telemetry documentation](https://go.dev/doc/telemetry#overview)). The variable is an implementation-level Go contract rather than a documented public `go env` setting, so a real re-exec/Go contract test should detect upstream changes.

## Additions, overrides, and invariants

The helper may accept explicit `NAME=value` additions because fixture commands need author identity, marker paths, and descendant `PATH` changes. It should parse into a map and sort only at the end. Reject entries without `=`, with an empty or invalid name, containing NUL, or repeating the same requested key; deterministic behavior should not rely on slice order or Go's “last duplicate wins” rule.

Overrides must not replace isolation or lifecycle invariants. Reserve:

- `HOME`, `TMPDIR`, `LANG`, `LC_ALL`, every XDG key created by the helper, and `GO_TELEMETRY_CHILD`;
- `GIT_CONFIG_GLOBAL`, `GIT_CONFIG_NOSYSTEM`, `GIT_TERMINAL_PROMPT`, `GIT_CONFIG_COUNT`, and every `GIT_CONFIG_KEY_*`/`GIT_CONFIG_VALUE_*` key;
- every Go-offline key listed in the next section when using that variant.

`PATH` is the only base key callers may explicitly replace, because some fixture descendants must resolve test stubs. Other names are additions, not overrides. Tests that need different `HOME`/XDG roots or a different Git/telemetry policy are not instances of the common contract and should keep a focused explicit environment at the call site.

## Go-offline variant

The variant extends the base; it does not inherit general Go variables from the host. It creates fresh writable directories under the same unique root and reserves these exact values:

```text
GOCACHE=<root>/go/build
GOMODCACHE=<root>/go/mod
GOPATH=<root>/go/path
GOENV=off
GONOPROXY=none
GOPROXY=file://<outer-GOMODCACHE>/cache/download
GOSUMDB=off
GOTOOLCHAIN=local
GOVCS=*:off
GOWORK=off
```

The host inputs are exactly `PATH` and the outer Go module *download cache*. Resolve the source cache before constructing the child environment: use a non-empty outer `GOMODCACHE` directly; otherwise run the absolute Go executable under `runtime.GOROOT()` with `GOENV=off`, `GOTOOLCHAIN=local`, `GO_TELEMETRY_CHILD=2`, outer `HOME` and `PATH`, and outer `GOPATH` when non-empty, then read `go env GOMODCACHE`. Require an absolute, clean result and convert it with `net/url`; it is a read source only. The child never receives the outer `GOCACHE`, writable module tree, `GOPATH`, Go environment file, workspace, proxy list, checksum database, or VCS policy.

This dependency path is necessary for `pack_documentation_test`: an empty `GOMODCACHE` plus `GOPROXY=off` cannot obtain Packy's external dependencies. The Go module reference explicitly permits a module cache's `cache/download` directory to serve as a `file://` proxy ([official `GOPROXY` reference](https://go.dev/ref/mod#environment-variables)). With a single local proxy URL and no comma/pipe fallback, missing artifacts fail locally. `GONOPROXY=none` prevents a private-pattern bypass to direct VCS; `GOVCS=*:off` denies VCS resolution; and `GOSUMDB=off` prevents checksum-database traffic. Existing `go.sum` entries still authenticate required module `.mod` and `.zip` bytes. `GOTOOLCHAIN=local` always uses the bundled toolchain and prevents automatic toolchain acquisition ([official Go toolchain documentation](https://go.dev/doc/toolchain#the-gotoolchain-setting)). `GOWORK=off` prevents discovery of a parent workspace.

The outer download cache is a provisioned build input, not ambient mutable configuration. A focused test may therefore fail with an actionable “module absent from local proxy” error if its dependency was not provisioned; it must never fall back to a network proxy or direct VCS. Fresh writable caches also mean parallel tests do not contend over cleanup state.

## Relationship to PR 719 and production architecture

PR #719 established two independent cleanup races: Go telemetry and Git automatic maintenance could outlive their direct parent and write while temporary roots were removed. Commit [`d442d75126a3ec07abb753f641e6125df52ef6f0`](https://github.com/yersonargotev/packy/commit/d442d75126a3ec07abb753f641e6125df52ef6f0) repaired the two owning production boundaries; the PR's final evidence passed both regressions 100 times and the affected packages under `-race` ([final PR evidence](https://github.com/yersonargotev/packy/pull/719#issuecomment-5382323116)).

Issue #724 is recurrence prevention for test-owned process setup. It is not the root fix for those production boundaries and must not consolidate their policies. ADR 0038 requires prepublication and mutation to remain distinct authority phases with bounded temporary state ([accepted decision](https://github.com/yersonargotev/packy/blob/bce18b693b1d2c789e263406e4ab37641bc8f321/docs/adr/0038-promote-releases-from-managed-pack-projects.md#L25-L39)). Keeping authority-phase, offline-validation, repository-candidate, bootstrap, and Claude Smoke product environments with their production owners preserves that decision.

No new ADR is required. `internal/testprocess` is test support with cohesive ownership and no product-facing contract. It deepens the test interface by hiding directory creation, allowlisting, invariant protection, validation, and sorting behind two environment operations while leaving every semantically important command choice visible.

## Approval-ready issue decisions

The issue should replace its current broad outcome and plan with these decisions:

1. **Scope.** Migrate test-owned command environments in `internal/ci`, `internal/managedpackpromotion/repositorycandidate`, `internal/managedpackpromotion/authorityphase`, `internal/tools/syncpacksource`, `internal/packsync`, `internal/cli`, and `internal/claudesmoke`. Include bare commands whose environment is completed by test code. Do not migrate a command whose environment is built by a production process boundary.
2. **Base.** Use the exact 18-key allowlist above, create every declared path, sort output, and never derive from `os.Environ` except for the single `PATH` value.
3. **Git.** Encode `maintenance.auto=false` and `gc.auto=0` with `GIT_CONFIG_COUNT/KEY/VALUE`; keep global/system/prompt protections. Do not add Git arguments or a command wrapper.
4. **Go.** Add only the focused offline variant above. Use isolated writable caches and the outer module download cache as a single `file://` dependency source with no network/VCS/toolchain fallback.
5. **Overrides.** Reject malformed/duplicate entries and invariant overrides; permit only `PATH` among base-key overrides.
6. **Exceptions.** Preserve `TestProjectVerifyRunsWithNoHomeOrPath` as an explicit minimal-environment proof. Preserve real production-environment probes and product-specific Claude Smoke allowlists.
7. **Ownership.** Do not change production process isolation in bootstrap, authority phase, offline validation, repository-candidate preparation/gates, or Claude Smoke. Do not add `TestMain`, process-global environment mutation, a generic command wrapper, or cleanup retries.

Approval-ready acceptance criteria are:

- Every in-scope real child has a non-nil environment from `testprocess`, except the named no-`HOME`/no-`PATH` re-exec test.
- The base environment contains exactly the documented invariant keys plus validated caller additions; all six declared directory variables point to existing unique real directories.
- Real Git observes `maintenance.auto=false` and `gc.auto=0`, does not read host global/system configuration, and cannot prompt for credentials.
- A real re-exec child and a Go child observe `GO_TELEMETRY_CHILD=2`; the #719 telemetry regression remains green 100 consecutive times.
- The Go-offline variant runs with fresh writable caches and succeeds from the local module download proxy while network is denied; a missing local module fails without fallback.
- Migrated tests do not use `t.Setenv`, `os.Environ`, or duplicate partial builders to control their children. This criterion is intentionally limited to migrated test-owned subprocesses; production-boundary tests such as bootstrap may still require separate production dependency injection.
- `exec.Command`, context, directory, I/O, arguments, network boundary, and expected output remain at their current call sites.
- Production environment code and its contract tests remain owned by their production modules.

## Validation plan

1. Add focused `internal/testprocess` contract tests for exact keys, sorted order, unique directories, mode/real-directory checks, malformed and duplicate entries, reserved overrides, allowed `PATH` replacement, and host-secret exclusion.
2. Run real Git against a poisoned outer global/system configuration and assert the two maintenance values plus the inability to observe the poison.
3. Re-exec the test binary and run a small Go child to assert the inherited base contract, especially `GO_TELEMETRY_CHILD=2`.
4. Run the no-dependency root-Go fixture and Pack documentation `go run` with fresh writable caches, the local `file://` proxy, and an enforced no-network boundary. Also assert that an absent module cannot trigger proxy, checksum-database, VCS, or toolchain network access.
5. Run the #719 telemetry and Git-maintenance regression tests 100 times, then the touched packages under `-race`.
6. Run `go test ./...` and `./scripts/validate-packy.sh --ci` with the outer `HOME` and `XDG_CONFIG_HOME` sandboxed, as required by repository guidance.

## Risks and limits

- **Scope drift:** a repository-wide “every child launched during tests” promise silently includes production boundaries. The ownership wording above is mandatory.
- **False offline claim:** `GOPROXY=off` with a fresh module cache cannot run `packdocs`; a local proxy source or vendored dependencies is mathematically required. Packy currently has no vendored upstream Go content, so the local download cache is the smallest existing capability.
- **Hidden override:** Git documents that explicit `git -c` options override `GIT_CONFIG_COUNT` pairs. Callers remain responsible for not negating the test invariant in their arguments unless that override is itself the behavior under test.
- **PATH semantics:** `cmd.Env` affects the child and descendants, not the earlier `exec.Command` lookup. Stub selection must use explicit executable paths instead of process-global `t.Setenv` in migrated tests.
- **Internal Go control:** `GO_TELEMETRY_CHILD=2` is source-backed but not a stable public `go env` setting. A live contract test is the guard against upstream change.
- **Outer module cache availability:** the offline variant is network-independent only after required artifacts are present in the outer download cache. Validation/CI must provision normal module dependencies before running the child; the child itself never provisions from the network.
- **Platform:** use `os.DevNull`, `os.PathListSeparator`, and `net/url`; do not encode `/dev/null`, `:`, or raw file URLs in the helper. Platform-specific sandbox enforcement remains at existing owners.
