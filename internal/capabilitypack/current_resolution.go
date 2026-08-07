package capabilitypack

import "fmt"

func (c Catalog) resolveIntentPack(id, version string) (Pack, error) {
	pack, err := c.Show(id)
	if err != nil {
		return Pack{}, err
	}
	if version != "" && version != pack.Version {
		return Pack{}, fmt.Errorf("capability pack %q receipt targets unsupported version %s; only catalog-current version %s is available", id, version, pack.Version)
	}
	return pack, nil
}

func (c Catalog) ResolveIntentPack(id, version string) (Pack, error) {
	return c.resolveIntentPack(id, version)
}

func (c Catalog) catalogEntry(id string) (catalogEntry, bool) {
	for _, entry := range c.entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return catalogEntry{}, false
}

func (c Catalog) validateUpdateRoute(id, _, toVersion string, _ int, _ Surface) error {
	pack, err := c.catalogMetadata(id)
	if err != nil {
		return err
	}
	if pack.Version != toVersion {
		return fmt.Errorf("capability pack %q can update only to catalog-current version %s", id, pack.Version)
	}
	return nil
}
