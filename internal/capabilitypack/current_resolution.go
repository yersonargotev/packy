package capabilitypack

import (
	"context"
	"fmt"
)

// IntentPackResolver resolves the immutable Pack contract recorded by an
// activation intent.
type IntentPackResolver func(context.Context, string, string, string) (Pack, error)

// IntentPackResolver snapshots catalog-current contracts so adapters can
// resolve ownership while the catalog observation lock is already held. Only
// receipts that reference a different, retained snapshot require storage.
func (c Catalog) IntentPackResolver() IntentPackResolver {
	current := make(map[string]Pack, len(c.packs))
	for _, pack := range c.packs {
		current[intentPackContractKey(pack.ID, pack.Version, pack.CatalogSnapshot)] = clonePack(pack)
	}
	return func(ctx context.Context, id, version, snapshotID string) (Pack, error) {
		if c.snapshotID != "" && snapshotID == "" {
			return Pack{}, fmt.Errorf("capability pack %q receipt predates Catalog Snapshots; follow the adoption procedure in docs/catalog-adoption.md before using the independent catalog", id)
		}
		if snapshotID != c.snapshotID {
			return c.resolveIntentPackAt(ctx, id, version, snapshotID)
		}
		pack, ok := current[intentPackContractKey(id, version, snapshotID)]
		if !ok {
			return Pack{}, fmt.Errorf("capability pack %q has no exact catalog-current ownership contract for version %q at Catalog Snapshot %q", id, version, snapshotID)
		}
		return clonePack(pack), nil
	}
}

func intentPackContractKey(id, version, snapshotID string) string {
	return id + "\x00" + version + "\x00" + snapshotID
}

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

// ResolveIntentDetailAt describes the exact immutable source recorded by an
// installed receipt, including a retained Catalog Snapshot.
func (c Catalog) ResolveIntentDetailAt(ctx context.Context, id, version, snapshotID string) (CatalogDetail, error) {
	pack, err := c.resolveIntentPackAt(ctx, id, version, snapshotID)
	if err != nil {
		return CatalogDetail{}, err
	}
	return catalogDetail(pack), nil
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
