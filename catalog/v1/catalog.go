package catalogv1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/mail"
	"net/url"
	"strings"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
)

// UpdateGraph is a named collection of bundles with upgrade relationships.
// It is the fundamental query primitive: callers use it to list available
// bundles and to ask "what can I upgrade to from here?"
type UpdateGraph interface {
	Name() string
	ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error]
	Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error]
	Property(ctx context.Context, key string) (json.RawMessage, error)
}

// Package is a package-root update graph with portable presentation metadata.
// Nested update graphs are channels and do not implement Package.
type Package interface {
	UpdateGraph
	Metadata(ctx context.Context) (PackageMetadata, error)
	Icon(ctx context.Context) (Icon, error)
}

// PackageMetadata contains portable package presentation metadata.
type PackageMetadata struct {
	DisplayName      string
	ShortDescription string
	Description      string
	Provider         Provider
	Maintainers      []Maintainer
	Keywords         []string
	SourceRepository *URL
	IconAvailable    bool
}

// Provider identifies the organization or person providing a package.
type Provider struct {
	Name string
	URL  *URL
}

// Maintainer identifies a package maintainer.
type Maintainer struct {
	Name  string
	Email *EmailAddress
}

// URL is a validated absolute HTTP or HTTPS URL.
type URL struct {
	value string
}

// ParseURL parses value as an absolute HTTP or HTTPS URL.
func ParseURL(value string) (URL, error) {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return URL{}, fmt.Errorf("%q is not an HTTP URL", value)
	}
	return URL{value: parsed.String()}, nil
}

func (u URL) String() string { return u.value }

func (u URL) MarshalText() ([]byte, error) { return []byte(u.value), nil }

func (u *URL) UnmarshalText(text []byte) error {
	parsed, err := ParseURL(string(text))
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

// EmailAddress is a validated email address without a display name.
type EmailAddress struct {
	value string
}

// ParseEmailAddress parses value as an email address without a display name.
func ParseEmailAddress(value string) (EmailAddress, error) {
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Name != "" || parsed.Address != value {
		return EmailAddress{}, fmt.Errorf("%q is not a valid email address", value)
	}
	return EmailAddress{value: value}, nil
}

func (e EmailAddress) String() string { return e.value }

func (e EmailAddress) MarshalText() ([]byte, error) { return []byte(e.value), nil }

func (e *EmailAddress) UnmarshalText(text []byte) error {
	parsed, err := ParseEmailAddress(string(text))
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}

// Icon is optional streamable package icon content. A zero Icon represents
// absence. Callers that receive content own closing it through Close.
type Icon struct {
	Content   io.ReadCloser
	MediaType string
}

// Close closes the icon content. Closing an absent icon is a no-op.
func (i Icon) Close() error {
	if i.Content == nil {
		return nil
	}
	return i.Content.Close()
}

// CompositeUpdateGraph is an UpdateGraph composed of named child UpdateGraphs.
// Catalog formats with channel-like concepts (e.g., FBC) implement this so
// callers can discover and query individual channels. Formats without channels
// return a plain UpdateGraph instead.
//
// ListBundles and Successors on a CompositeUpdateGraph operate across all
// child graphs (the union).
type CompositeUpdateGraph interface {
	UpdateGraph
	ListGraphs(ctx context.Context) iter.Seq2[UpdateGraph, error]
	GetGraph(ctx context.Context, name string) (UpdateGraph, error)
}

// Deprecated is implemented by UpdateGraph and Bundle values that carry a
// deprecation message. Callers discover deprecation via type assertion.
type Deprecated interface {
	DeprecationMessage() string
}

// Catalog is the top-level entry point for querying a catalog.
// Implementations backed by formats with channel concepts (e.g., FBC) may
// return Package values that also implement CompositeUpdateGraph.
type Catalog interface {
	Name() string
	URI() string
	Digest() string
	Priority() int
	Labels() map[string]string

	ListPackages(ctx context.Context) iter.Seq2[Package, error]
	GetPackage(ctx context.Context, name string) (Package, error)
}
