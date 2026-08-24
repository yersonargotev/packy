package capabilitypack

import (
	"context"
	"fmt"
)

func (c Catalog) resolveIntentPack(ctx context.Context, id, version string) (Pack, error) {
	pack, err := c.Show(ctx, id)
	if err != nil {
		return Pack{}, err
	}
	if version != "" && version != pack.Version {
		return Pack{}, fmt.Errorf("capability pack %q receipt targets unsupported version %s; only catalog-current version %s is available", id, version, pack.Version)
	}
	return pack, nil
}

func (c Catalog) ResolveIntentPack(ctx context.Context, id, version string) (Pack, error) {
	return c.resolveIntentPack(ctx, id, version)
}

func (c Catalog) validateUpdateRoute(id, _, toVersion string, _ Surface) error {
	pack, err := c.catalogMetadata(id)
	if err != nil {
		return err
	}
	if pack.Version != toVersion {
		return fmt.Errorf("capability pack %q can update only to catalog-current version %s", id, pack.Version)
	}
	return nil
}
