# Pack authoring template

Copy this directory to `bundle/packs/<pack-id>`. A Pack has reviewed content and
one `pack.json` manifest.

1. Set the Pack identity, description, selectability, supported surfaces, and
   maintainer-selected SemVer in `pack.json`.
2. Replace the example with the Pack's reviewed content beneath its Pack
   directory.
3. Point each resource `source` at that reviewed content.
4. Declare only intra-Pack dependencies, external requirements, concrete
   conflicts, bindings, and intentional exclusions.
5. Run the focused validator:

   ```sh
   ./scripts/validate-pack-content.sh <pack-id>
   ```

Review the content and manifest together. The checked-in Pack is the complete
authoring authority.
