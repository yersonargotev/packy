package capabilitypack

import "context"

// withCatalogObservation keeps catalog selection, historical resolution, and
// adapter reads on one complete catalog generation.
func withCatalogObservation[T any](ctx context.Context, facade Facade, observe func(Facade) (T, error)) (T, error) {
	var result T
	err := facade.catalog.withCatalogLock(ctx, func(locked Catalog) error {
		fresh, err := locked.refreshed(ctx)
		if err != nil {
			return err
		}
		facade.catalog = fresh
		result, err = observe(facade)
		return err
	})
	return result, err
}
