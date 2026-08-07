# Pack authoring template

Copy this directory to `bundle/packs/<pack-id>`, then update `pack.json`:

1. Set the Pack identity, description, selectability, supported surfaces, and maintainer-selected SemVer.
2. Change resource `source` paths to their reviewed paths beneath `bundle/`.
3. Declare only intra-Pack `requires`, external tool requirements, concrete resource conflicts, bindings, and intentional exclusions.
4. Remove `source_reference` when no informational upstream reference is useful.
5. Run `./scripts/validate-pack-content.sh <pack-id>`.

The reviewed files in this repository are authoritative. Source references do not authorize synchronization or admission.
