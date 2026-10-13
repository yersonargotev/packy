// Package catalogorigin resolves exact public GitHub origins for Catalog
// Project validation commands.
package catalogorigin

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/yersonargotev/packy/internal/cataloglayout"
)

// Resolver gives one invocation private copies of verified retained origins.
type Resolver struct {
	temporary string
	resolved  map[string]string
	caBundle  []byte
}

// New creates an empty origin resolver.
func New() (*Resolver, error) {
	// Honor the explicit certificate bundle on every host, including systems
	// whose native Go trust store does not read SSL_CERT_FILE itself.
	var caBundle []byte
	if path := os.Getenv("SSL_CERT_FILE"); path != "" {
		var err error
		caBundle, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read origin SSL_CERT_FILE: %w", err)
		}
		if !x509.NewCertPool().AppendCertsFromPEM(caBundle) {
			return nil, fmt.Errorf("origin SSL_CERT_FILE contains no PEM certificates")
		}
	}
	temporary, err := os.MkdirTemp("", "packy-catalog-origins-")
	if err != nil {
		return nil, fmt.Errorf("create temporary origin directory: %w", err)
	}
	return &Resolver{temporary: temporary, resolved: map[string]string{}, caBundle: caBundle}, nil
}

// Close removes the resolver's owned temporary checkouts.
func (r *Resolver) Close() error {
	if r == nil || r.temporary == "" {
		return nil
	}
	return os.RemoveAll(r.temporary)
}

// Resolve returns a private verified checkout of one exact public GitHub origin.
func (r *Resolver) Resolve(ctx context.Context, origin cataloglayout.Origin) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	key := origin.Repository + "\x00" + origin.Commit
	if root := r.resolved[key]; root != "" {
		return root, nil
	}
	root, err := r.resolve(ctx, origin)
	if err == nil {
		r.resolved[key] = root
	}
	return root, err
}
