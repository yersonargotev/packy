package capabilitypack

import (
	"context"
	"fmt"
)

// IntentPackResolver resolves the immutable Pack contract recorded by an
// activation intent.
type IntentPackResolver func(context.Context, string, string, CatalogRef) (Pack, error)

// IntentPackResolver snapshots catalog-current contracts so adapters can
// resolve ownership while the catalog observation lock is already held. Only
// receipts that reference a different, retained snapshot require storage.
func (c Catalog) IntentPackResolver() IntentPackResolver {
	current := make(map[intentPackContract]Pack, len(c.packs))
	for _, pack := range c.packs {
		current[intentPackContract{pack.ID, pack.Version, pack.CatalogRef}] = clonePack(pack)
	}
	return func(ctx context.Context, id, version string, ref CatalogRef) (Pack, error) {
		if c.ref != (CatalogRef{}) && ref == (CatalogRef{}) {
			return Pack{}, unboundReceiptError(id)
		}
		if ref != c.ref {
			pack, err := c.resolveIntentPackAt(ctx, id, version, ref)
			if err != nil {
				return Pack{}, err
			}
			if pack.ID != id || pack.Version != version || pack.CatalogRef != ref {
				return Pack{}, fmt.Errorf("capability pack %q has no exact retained ownership contract for version %q at Catalog Snapshot %s", id, version, ref.label())
			}
			return pack, nil
		}
		pack, ok := current[intentPackContract{id, version, ref}]
		if !ok {
			return Pack{}, fmt.Errorf("capability pack %q has no exact catalog-current ownership contract for version %q at Catalog Snapshot %s", id, version, ref.label())
		}
		return clonePack(pack), nil
	}
}

type intentPackContract struct {
	id, version string
	ref         CatalogRef
}

func unboundReceiptError(id string) error {
	return fmt.Errorf("capability pack %q receipt names no catalog; follow the adoption procedure in docs/catalog-adoption.md to reinstall it", id)
}

func (c Catalog) resolveIntentPack(ctx context.Context, id, version string) (Pack, error) {
	return c.resolveIntentPackAt(ctx, id, version, CatalogRef{})
}

func (c Catalog) resolveIntentPackAt(ctx context.Context, id, version string, ref CatalogRef) (Pack, error) {
	if c.ref != (CatalogRef{}) && ref == (CatalogRef{}) {
		return Pack{}, unboundReceiptError(id)
	}
	if ref != (CatalogRef{}) && ref != c.ref {
		if c.resolveSnapshot == nil {
			return Pack{}, fmt.Errorf("capability pack %q receipt references unavailable Catalog Snapshot %s", id, ref.label())
		}
		catalogRoot, err := c.resolveSnapshot(ctx, ref)
		if err != nil {
			return Pack{}, fmt.Errorf("resolve Catalog Snapshot %s for capability pack %q: %w", ref.label(), id, err)
		}
		if err := ctx.Err(); err != nil {
			return Pack{}, err
		}
		// resolveSnapshot validates the retained immutable snapshot before
		// returning its catalog root. Read those bytes directly: acquiring the
		// retained catalog lock while the current catalog lock is held would allow
		// two cross-snapshot observations to deadlock in opposite order.
		historical, err := discoverRetainedForDurableIntentsUnlocked(catalogRoot, ref, c.resolveSnapshot)
		if err != nil {
			return Pack{}, fmt.Errorf("load retained Catalog Snapshot %s: %w", ref.label(), err)
		}
		pack, err := historical.showUnlocked(id)
		if err != nil {
			return Pack{}, err
		}
		if version != "" && version != pack.Version {
			return Pack{}, fmt.Errorf("capability pack %q receipt version %s does not match retained Catalog Snapshot %s version %s", id, version, ref.label(), pack.Version)
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
func (c Catalog) ResolveIntentPackAt(ctx context.Context, id, version string, ref CatalogRef) (Pack, error) {
	return c.resolveIntentPackAt(ctx, id, version, ref)
}

// ResolveIntentDetailAt describes the exact immutable source recorded by an
// installed receipt, including a retained Catalog Snapshot.
func (c Catalog) ResolveIntentDetailAt(ctx context.Context, id, version string, ref CatalogRef) (CatalogDetail, error) {
	pack, err := c.resolveIntentPackAt(ctx, id, version, ref)
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
