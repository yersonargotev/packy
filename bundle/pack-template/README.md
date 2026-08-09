# Pack authoring template

Copy this directory to `bundle/packs/<pack-id>`. A Pack has reviewed content and
one `pack.json` manifest.

1. Set the Pack identity, description, selectability, supported surfaces,
   readiness obligations, and maintainer-selected SemVer in `pack.json`. Keep
   the required obligations as the sorted set `runtime-usability` and
   `surface-authorization`.
2. Replace the example with the Pack's reviewed content beneath its Pack
   directory.
3. Give every resource a concise `description` of its purpose within the Pack.
   Do not merely repeat source paths or projection mechanics.
4. Point each resource `source` at that reviewed content.
5. Declare only intra-Pack dependencies, external requirements, concrete
   conflicts, bindings, reviewed typed surface capabilities, and intentional
   exclusions. Every binding requires a `capabilities` array; use `[]` when it
   requests no additional host-native behavior.
6. Run the focused validator:

   ```sh
   ./scripts/validate-pack-content.sh <pack-id>
   ```

Review the content and manifest together. The checked-in Pack is the complete
authoring authority.
