package internal

import (
	"context"
	"database/sql"
	"fmt"
)

// PackageSchemaHandler normalizes a package and its companion blobs
// from the raw tables into the normalized tables.
type PackageSchemaHandler interface {
	// Schema returns the package schema this handler processes (e.g. "olm.package").
	Schema() string

	// Normalize validates the package's neighborhood in the raw tables
	// and populates the normalized tables (bundles, graphs, successor edges).
	// Called once per package during the normalization phase.
	Normalize(ctx context.Context, tx *sql.Tx, packageName string) error
}

// HandlerRegistry maps package schema strings to their handlers.
type HandlerRegistry struct {
	handlers map[string]PackageSchemaHandler
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{handlers: make(map[string]PackageSchemaHandler)}
}

func (r *HandlerRegistry) Register(h PackageSchemaHandler) {
	r.handlers[h.Schema()] = h
}

func (r *HandlerRegistry) Get(schema string) (PackageSchemaHandler, error) {
	h, ok := r.handlers[schema]
	if !ok {
		return nil, fmt.Errorf("no handler registered for package schema %q", schema)
	}
	return h, nil
}
