# Opt-in skill-template acceptance

These probes exercise real reviewed Catalog content and installed host binaries.
They are not part of ordinary Go tests or CI. They never publish a Catalog,
execute a skill helper, use user credentials, or modify user installation state.
The complete Catalog build resolves exact upstream sources over the network.

Build the implementation under review with its local source identity:

```sh
go build -ldflags "-X github.com/yersonargotev/packy/internal/version.Value=v0.0.0-skill-template-probe -X github.com/yersonargotev/packy/internal/version.SourceCommit=$(git rev-parse HEAD)" \
  -o /tmp/packy-skill-template-probe ./cmd/packy
python3 -B scripts/probes/skill-template-catalog.py \
  --catalog /path/to/reviewed/packy-catalog \
  --packy /tmp/packy-skill-template-probe \
  --out-dir /tmp/new-packy-skill-template-evidence
```

The supplied Catalog checkout must contain the reviewed Matty 4.0.0 and pstack
4.3.0 baseline and its existing bounded `content-probes.py` candidate runner.
The output directory must be new. The probe preserves the checkout, copies its
complete Pack set, and authors one isolated pstack 5.0.0 candidate adaptation:
namespace `tdd` and `teach` on each surface, render the Codex help's three files,
and declare its direct TDD/teaching dependencies. This version is a local
candidate label, not a published Pack release or release request.

It builds the original and adapted Catalog Snapshots and, on all three surfaces
in project and global scope, verifies original complete-Pack collisions, failure
of directory aliases, read-only availability transition, full adapted coexistence,
correct native frontmatter, pstack-local help routes, and preservation of Matty.
Successful workspaces remain under the owned output directory for inspection;
project installations additionally pass portable `verify`. Ordinary CLI tests
cover both installation orders, lifecycle update/removal, drift, stale plans,
explicit aliases, and adoption of generated naming by an already installed Pack.

To check the installed real hosts against those workspaces:

```sh
python3 -B scripts/probes/skill-template-hosts.py \
  --catalog-evidence /tmp/new-packy-skill-template-evidence/evidence.json
```

Codex uses `initialize`, `initialized`, and `skills/list` through the installed
app-server to verify all four native identities without a thread or model turn,
following the [official protocol](https://learn.chatgpt.com/docs/app-server).
Claude requests each explicit namespaced command against a localhost recorder
that returns HTTP 400 after observing pre-inference skill expansion. A nonzero
Claude exit is expected after that recorded expansion. OpenCode uses a localhost
deterministic provider to request its actual skill-loading tool, then stop.
Neither transport performs model inference. OpenCode's ordinary compatibility
roots remain enabled; the probe does not claim persistent launch isolation.

The host probe uses HOME/config/data/state/cache/temp directories beneath each
owned candidate workspace, with an explicit minimal environment and localhost
fixture credentials. It records actual versions, identities, and body-loading
results in `host-evidence.json`. This validates native discovery/tool loading,
not the semantic success of an arbitrary model-driven engineering workflow.
