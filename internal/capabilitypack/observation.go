package capabilitypack

import "context"

// withBundleObservation keeps catalog selection, historical resolution, and
// adapter reads on one complete bundle generation.
func withBundleObservation[T any](ctx context.Context, facade Facade, observe func(Facade) (T, error)) (T, error) {
	var result T
	err := facade.catalog.withBundleLock(ctx, func(locked Catalog) error {
		fresh, err := locked.refreshed()
		if err != nil {
			return err
		}
		facade.catalog = fresh
		result, err = observe(facade)
		return err
	})
	return result, err
}
