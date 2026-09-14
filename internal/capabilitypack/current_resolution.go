package capabilitypack

import (
	"context"
	"fmt"
)

func (c Catalog) resolveIntentPack(ctx context.Context, id, version string) (Pack, error) {
	return c.resolveIntentPackAt(ctx, id, version, "")
}

func (c Catalog) resolveIntentPackAt(ctx context.Context, id, version, snapshotID string) (Pack, error) {
	if c.snapshotID != "" && snapshotID == "" {
		return Pack{}, fmt.Errorf("capability pack %q receipt predates Catalog Snapshots; follow the adoption procedure in docs/catalog-adoption.md before using the independent catalog", id)
	}
	if snapshotID != "" && snapshotID != c.snapshotID {
		if c.resolveSnapshot == nil {
			return Pack{}, fmt.Errorf("capability pack %q receipt references unavailable Catalog Snapshot %s", id, snapshotID)
		}
		bundleRoot, err := c.resolveSnapshot(ctx, snapshotID)
		if err != nil {
			return Pack{}, fmt.Errorf("resolve Catalog Snapshot %s for capability pack %q: %w", snapshotID, id, err)
		}
		historical, err := DiscoverRetainedForDurableIntents(ctx, bundleRoot, snapshotID, c.resolveSnapshot)
		if err != nil {
			return Pack{}, fmt.Errorf("load retained Catalog Snapshot %s: %w", snapshotID, err)
		}
		pack, err := historical.Show(ctx, id)
		if err != nil {
			return Pack{}, err
		}
		if version != "" && version != pack.Version {
			return Pack{}, fmt.Errorf("capability pack %q receipt version %s does not match retained Catalog Snapshot %s version %s", id, version, snapshotID, pack.Version)
		}
		return pack, nil
	}
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

// ResolveIntentPackAt resolves the exact immutable source recorded by an
// installed receipt, including a retained Catalog Snapshot.
func (c Catalog) ResolveIntentPackAt(ctx context.Context, id, version, snapshotID string) (Pack, error) {
	return c.resolveIntentPackAt(ctx, id, version, snapshotID)
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
