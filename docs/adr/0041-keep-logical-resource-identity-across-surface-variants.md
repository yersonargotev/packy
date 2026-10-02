---
status: accepted
---

# Keep logical resource identity across reviewed surface variants

Model one host-independent capability as one Pack resource with a common
definition and typed per-surface overrides instead of separate host-specific
resource identities. This preserves selection and dependency identity while
allowing every resource kind to adapt content and execution to its host.
Resolve the effective resource before graph closure and lifecycle planning.
Missing variants inherit the reviewed common definition; invalid declared
variants fail without silent fallback.

Surface bindings remain the host-native projection and support boundary.
Variants do not change kind, ID, bindings, or exclusions, authorize an excluded
host, or introduce executable adaptation logic. The optional `variants` array
is sorted and unique by surface. Its typed fields replace explicitly present
values, including whole arrays and objects. Omission inherits; null is invalid.
The applicable existing source, provenance, dependencies, notices, description,
execution, authority, and attribution fields retain their resource-kind rules.

All reviewed bodies remain in declared source and legal closure, including an
unused original. A changed imported source requires explicit variant provenance;
exact-copy originals and adapted bodies retain distinct relationships. The
Declared Pack Closure is the deterministic union of common, variant, and typed
binding-capability roots. This refines ADR 0040's common-resource/source model
while retaining host-independent identity and Pack-relative source constraints.

Separate host directories are necessary where physical paths overlap, but are
insufficient when hosts also discover compatibility directories. Host coexistence
requires verified native discovery for supported host versions and launch
contexts. Unsupported or unverified divergent same-name combinations block before
mutation. Packy does not infer persistent isolation from its own environment or
silently edit shell or application launch settings.

Separate resource IDs fragment selection and dependencies; separate Packs also
fragment versions and provenance. Automatic prompt rewriting would introduce
unreviewed installation-time adaptation. Explicit reviewed variants keep content
adaptation in the canonical Catalog Project with one typed engine resolver.

Manifest schema v3 and Catalog Snapshot index v3 are a clean cut. Older formats
are rejected rather than converted. Release the engine first, then adopt and
publish the complete catalog using that immutable engine revision. Existing
installations follow the explicit deactivation/reinstallation process. Shared
logical identity does not imply identical authority, dependencies, or runtime
usability. The first engine milestone demonstrates complete skill lifecycle;
all-kind acceptance and verified coexistence remain separately proven slices.
