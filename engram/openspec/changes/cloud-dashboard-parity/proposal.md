# Proposal: Restore cloud dashboard parity

## Intent

The integrated repo only exposes a minimal sync-status dashboard, while `/Users/alanbuscaglia/work/engram-cloud` already contains the proven browser dashboard. Copy/reconcile that implementation first to preserve behavior and avoid redesign.

## Scope

### In Scope
- Restore dashboard routes, templates, partials, assets, and tests from `engram-cloud` into `internal/cloud/dashboard`.
- Reconcile missing `cloudstore` query/index APIs needed by overview, browser, projects, contributors, and admin pages.
- Adapt auth/runtime wiring to integrated constraints (`ENGRAM_CLOUD_TOKEN`, signed dashboard cookie, `ENGRAM_JWT_SECRET`, admin config).
- Update docs for restored behavior and runtime requirements.

### Out of Scope
- IA/styling redesign beyond compatibility fixes.
- Replacing token-based cloud auth with a new identity model.
- Broad cloudstore/schema changes unrelated to parity.

## Capabilities

### New Capabilities
- `cloud-dashboard`: authenticated server-rendered cloud dashboard with htmx-enhanced browsing, project/admin views, and parity with the standalone cloud repo.

### Modified Capabilities
- None.

## Approach

Use source-guided restoration, not fresh invention.
- Phase 1: port layout/components/routes/assets/tests, keeping server-rendered URLs and form fallback.
- Phase 2: add missing `cloudstore` read models/indexes required by restored handlers.
- Phase 3: adapt auth/runtime seams: translate JWT-cookie assumptions to the integrated bearer-token session codec, preserve `/dashboard/login` POST fallback, and honor `ENGRAM_CLOUD_ADMIN` plus `ENGRAM_JWT_SECRET` rules.
- Phase 4: align docs and add parity regression tests for dashboard auth plus cloud push/pull boundaries.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/cloud/dashboard/**` | Modified | Replace status-only dashboard with restored routes/templates. |
| `internal/cloud/cloudstore/cloudstore.go` | Modified | Add dashboard read/query surfaces. |
| `internal/cloud/cloudserver/cloudserver.go` | Modified | Mount full dashboard and reconcile auth/session wiring. |
| `cmd/engram/cloud.go`, `internal/cloud/config.go` | Modified | Support restored admin/auth runtime behavior. |
| `README.md`, `DOCS.md`, `docs/*` | Modified | Document real dashboard behavior and setup. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Source/target auth mismatch | High | Keep existing session codec boundary; adapt handlers around it. |
| Hidden schema/query gaps | High | Port in phases, starting with read-model APIs backed by tests. |
| UI drift from current routes | Med | Preserve shareable URLs and non-htmx form fallback in parity tests. |

## Rollback Plan

Revert dashboard route mounting and `dashboard/cloudstore` deltas, returning to the current status-only dashboard while leaving sync/auth contracts unchanged.

## Dependencies

- Source implementation in `/Users/alanbuscaglia/work/engram-cloud/internal/cloud/dashboard`
- Existing integrated auth/session codec in `internal/cloud/{auth,cloudserver}`

## Success Criteria

- [ ] Integrated repo serves the same dashboard surface area as `engram-cloud` without redefining product behavior.
- [ ] Dashboard auth works with the integrated runtime and redirects/fallback forms behave without htmx.
- [ ] Required dashboard read models exist with regression coverage.
- [ ] Docs reflect actual restored setup and runtime requirements.
