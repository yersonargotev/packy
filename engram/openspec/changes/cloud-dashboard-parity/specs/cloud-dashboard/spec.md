# cloud-dashboard Specification

## Purpose

Restore standalone `engram-cloud` dashboard behavior in the integrated repo while preserving local-first and current security/runtime rules.

## Requirements

### Requirement: Route and navigation parity

The system MUST expose authenticated, shareable URLs for overview, browser, projects, contributors, and admin with parity to standalone cloud behavior.

#### Scenario: Direct URL access works after login
- GIVEN an authenticated dashboard session
- WHEN the user opens a deep dashboard URL
- THEN the server renders the corresponding page
- AND navigation state is preserved in the URL

#### Scenario: Unauthenticated access is redirected
- GIVEN no valid dashboard session
- WHEN any protected dashboard route is requested
- THEN the response redirects to `/dashboard/login`

### Requirement: Server-rendered pages with htmx enhancement

The system SHALL use server-rendered HTML as primary output, and htmx interactions MUST degrade to normal HTTP GET/POST behavior.

#### Scenario: htmx partial request
- GIVEN an authenticated user on a dashboard page
- WHEN a partial endpoint is requested via htmx
- THEN the response contains meaningful standalone HTML

#### Scenario: Non-htmx form fallback
- GIVEN htmx is unavailable
- WHEN the user submits a dashboard form
- THEN the request succeeds via standard HTTP semantics

### Requirement: Static asset parity

The system MUST serve dashboard static assets (styles, scripts, icons) required for restored layout parity.

#### Scenario: Styled login page loads assets
- GIVEN a request to `/dashboard/login`
- WHEN the page is rendered
- THEN required assets are served successfully

### Requirement: Browser and projects data surfaces

The system MUST provide cloudstore-backed read models for browser and project views without changing local SQLite source-of-truth semantics.

#### Scenario: Browser view shows replicated cloud records
- GIVEN replicated cloud data exists
- WHEN the browser view is requested
- THEN results reflect cloud read models
- AND no local-first ownership rules are bypassed

#### Scenario: Project view details are queryable
- GIVEN a project with replicated metadata and contributors
- WHEN the project detail page is requested
- THEN the page renders project-specific cloud-backed data

### Requirement: Contributors and admin surfaces

The system MUST restore contributors and admin surfaces with access controls aligned to current runtime roles and admin configuration.

#### Scenario: Admin-only view enforcement
- GIVEN an authenticated non-admin dashboard user
- WHEN the admin surface is requested
- THEN access is denied per current security policy

#### Scenario: Contributor surface availability
- GIVEN an authenticated authorized user
- WHEN the contributors surface is requested
- THEN contributor information renders from supported cloudstore read models

### Requirement: Login and dashboard session flow adaptation

The system MUST preserve a styled `/dashboard/login` flow and adapt auth to integrated runtime boundaries (`ENGRAM_CLOUD_TOKEN`, signed dashboard cookie, `ENGRAM_JWT_SECRET` validation).

#### Scenario: Login POST fallback creates session
- GIVEN valid runtime auth configuration
- WHEN credentials or token material are posted to `/dashboard/login`
- THEN a valid dashboard session is established
- AND protected dashboard routes become accessible

#### Scenario: Invalid runtime secret configuration
- GIVEN `ENGRAM_JWT_SECRET` is missing or invalid
- WHEN dashboard auth/session validation occurs
- THEN login/session establishment fails safely

### Requirement: Security and local-first invariants

The system MUST NOT weaken existing security controls and MUST keep local SQLite as source of truth, treating cloud data as replication/shared-access views.

#### Scenario: Sync boundary preserved
- GIVEN dashboard parity features are enabled
- WHEN push and pull sync operations execute
- THEN sync contracts remain valid
- AND regression tests cover both directions

### Requirement: Documentation parity

The system MUST update docs to reflect restored routes, runtime variables, auth/session behavior, and non-htmx fallback.

#### Scenario: Operator follows docs to enable dashboard
- GIVEN a clean integrated setup
- WHEN an operator follows documented instructions
- THEN they can configure runtime and reach the restored dashboard surfaces as documented
