package catalogv1

import (
	"context"

	"k8s.io/apimachinery/pkg/labels"
)

// Writer provides methods for inserting catalog content into a store.
type Writer interface {
	InsertBundle(id, pkg, version, release, uri string) error
	CreateGraph(path []string) error
	AddBundleToGraph(path []string, bundleID string) error
	AddEdge(path []string, fromBundleID, toBundleID string) error
	AddPredecessorRange(path []string, bundleID, versionRange string) error
	SetBundleProperty(bundleID, key string, val any) error
	SetGraphProperty(path []string, key string, val any) error
	SetGraphDeprecation(path []string, message string) error
	SetBundleDeprecation(bundleID string, message string) error
}

// Importer defines how catalog content is imported into a store via a Writer.
type Importer interface {
	Import(ctx context.Context, w Writer) error
}

// PartialImportError is a marker interface for errors that indicate a partial
// import: some content was successfully written, but per-item errors occurred.
// When Import returns a PartialImportError, Set commits the transaction and
// propagates the error to the caller alongside the Catalog value.
type PartialImportError interface {
	error
	PartialImport()
}

// StoreReader is the read-only subset of Store. A StoreReader obtained via
// Select shares the underlying storage with its parent Store and is only
// valid while the parent Store remains open.
type StoreReader interface {
	Get(name string) (Catalog, error)
	List() ([]Catalog, error)
	Select(selector labels.Selector) StoreReader
}

// Store manages a collection of named catalogs backed by persistent storage.
//
// Catalog values returned by Get and List are snapshots: their metadata
// (Name, URI, Digest, Priority, Labels) reflects the state at query time.
// Subsequent Set calls do not update previously returned Catalog values.
// Call Get again to obtain fresh metadata. Content queries (ListPackages,
// GetPackage) always read from the underlying database on demand.
type Store interface {
	StoreReader
	Set(ctx context.Context, name string, opts ...SetOption) (Catalog, error)
	Delete(name string) error
	Close() error
}

// SetOption configures a Set operation.
type SetOption func(*SetConfig)

// SetConfig holds the resolved configuration for a Set operation.
type SetConfig struct {
	URI      *string
	Priority *int
	Labels   *map[string]string
	Content  *ContentConfig
}

// ContentConfig holds the content importer and digest for a Set operation.
type ContentConfig struct {
	Importer Importer
	Digest   string
}

// ApplySetOptions applies the given options and returns the resulting config.
func ApplySetOptions(opts []SetOption) SetConfig {
	var cfg SetConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithURI sets the URI for the catalog entry.
func WithURI(uri string) SetOption {
	return func(c *SetConfig) {
		c.URI = &uri
	}
}

// WithPriority sets the priority for the catalog entry.
func WithPriority(priority int) SetOption {
	return func(c *SetConfig) {
		c.Priority = &priority
	}
}

// WithLabels sets the labels for the catalog entry.
func WithLabels(labels map[string]string) SetOption {
	return func(c *SetConfig) {
		c.Labels = &labels
	}
}

// WithContent sets the content importer and digest for the catalog entry.
func WithContent(importer Importer, digest string) SetOption {
	return func(c *SetConfig) {
		c.Content = &ContentConfig{
			Importer: importer,
			Digest:   digest,
		}
	}
}
