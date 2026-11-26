package githubfake

import (
	"fmt"
	"os"
	"path/filepath"
)

// Pack names one Pack of a fixture Catalog Project.
type Pack struct {
	ID      string
	Version string
}

// WriteCatalogProject writes a Catalog Project whose Packs each carry one
// agents skill, ready for packy catalog build.
func WriteCatalogProject(root string, packs ...Pack) error {
	for _, pack := range packs {
		packRoot := filepath.Join(root, "packs", pack.ID)
		skill := filepath.Join(packRoot, "skills", pack.ID, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
			return err
		}
		body := fmt.Sprintf("---\nname: %s\ndescription: Fixture skill of %s@%s.\n---\n\nFixture skill.\n", pack.ID, pack.ID, pack.Version)
		if err := os.WriteFile(skill, []byte(body), 0o644); err != nil {
			return err
		}
		manifest := fmt.Sprintf(`{
  "schema_version": 3,
  "id": %[1]q,
  "version": %[2]q,
  "description": "Fixture Pack",
  "selectable": true,
  "surfaces": ["agents"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [{
    "kind": "skill",
    "id": %[1]q,
    "source": %[3]q,
    "description": "Fixture skill",
    "requires": [],
    "conflicts": [],
    "bindings": [{
      "surface": "agents",
      "projection": "skill",
      "name": %[1]q,
      "invocation": %[4]q,
      "mode": "native",
      "sharing": "exclusive",
      "capabilities": []
    }],
    "surface_exclusions": []
  }]
}
`, pack.ID, pack.Version, "skills/"+pack.ID, "$"+pack.ID)
		if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), []byte(manifest), 0o644); err != nil {
			return err
		}
	}
	return nil
}
