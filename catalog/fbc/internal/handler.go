package internal

import (
	"context"
	"database/sql"
	"fmt"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// PackageSchemaHandler normalizes a package and its companion blobs
// from the raw tables into the normalized content via a Writer.
type PackageSchemaHandler interface {
	// Schema returns the package schema this handler processes (e.g. "olm.package").
	Schema() string

	// Normalize validates the package's neighborhood in the raw tables
	// and writes normalized content (bundles, graphs, successor edges)
	// through the Writer. The rawDB is used for reading raw staging tables.
	// Called once per package during the normalization phase.
	Normalize(ctx context.Context, rawDB *sql.DB, w catalogv1.Writer, packageName string) error
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
