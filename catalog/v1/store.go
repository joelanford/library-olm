package catalogv1

import "context"

// GraphID is the database identifier for a graph node.
type GraphID int64

// Writer provides methods for inserting catalog content into a store.
type Writer interface {
	InsertBundle(id, pkg, version, release, uri string) error
	CreateGraph(name string, parent *GraphID) (GraphID, error)
	AddBundleToGraph(graph GraphID, bundleID string) error
	AddSuccessor(graph GraphID, fromBundleID, toBundleID string) error
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

// Store manages a collection of named catalogs backed by persistent storage.
type Store interface {
	Set(ctx context.Context, name string, opts ...SetOption) (Catalog, error)
	Get(name string) (Catalog, error)
	Delete(name string) error
	List() ([]Catalog, error)
	Close() error
}

// SetOption configures a Set operation.
type SetOption func(*setConfig)

type setConfig struct {
	uri      *string
	priority *int
	labels   *map[string]string
	content  *contentConfig
}

type contentConfig struct {
	importer Importer
	digest   string
}

// WithURI sets the URI for the catalog entry.
func WithURI(uri string) SetOption {
	return func(c *setConfig) {
		c.uri = &uri
	}
}

// WithPriority sets the priority for the catalog entry.
func WithPriority(priority int) SetOption {
	return func(c *setConfig) {
		c.priority = &priority
	}
}

// WithLabels sets the labels for the catalog entry.
func WithLabels(labels map[string]string) SetOption {
	return func(c *setConfig) {
		c.labels = &labels
	}
}

// WithContent sets the content importer and digest for the catalog entry.
func WithContent(importer Importer, digest string) SetOption {
	return func(c *setConfig) {
		c.content = &contentConfig{
			importer: importer,
			digest:   digest,
		}
	}
}
