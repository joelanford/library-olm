package api

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
)

type resourceLink struct {
	Href string `json:"href"`
}

type catalogSummaryLinks struct {
	Self resourceLink `json:"self"`
}

type catalogDetailLinks struct {
	Self resourceLink `json:"self"`
}

type packageSummaryLinks struct {
	Self    resourceLink `json:"self"`
	Catalog resourceLink `json:"catalog"`
}

type packageDetailLinks struct {
	Self     resourceLink  `json:"self"`
	Catalog  resourceLink  `json:"catalog"`
	Channels resourceLink  `json:"channels"`
	Bundles  resourceLink  `json:"bundles"`
	Icon     *resourceLink `json:"icon,omitempty"`
}

type channelSummaryLinks struct {
	Self resourceLink `json:"self"`
}

type channelDetailLinks struct {
	Self    resourceLink `json:"self"`
	Package resourceLink `json:"package"`
	Bundles resourceLink `json:"bundles"`
}

type bundleSummaryLinks struct {
	Self resourceLink `json:"self"`
}

type bundleDetailLinks struct {
	Self    resourceLink `json:"self"`
	Package resourceLink `json:"package"`
	Catalog resourceLink `json:"catalog"`
}

type catalogSummary struct {
	Name     string              `json:"name"`
	URI      string              `json:"uri"`
	Digest   string              `json:"digest"`
	Priority int                 `json:"priority"`
	Labels   map[string]string   `json:"labels"`
	Links    catalogSummaryLinks `json:"links"`
}

type catalogDetail struct {
	Name     string             `json:"name"`
	URI      string             `json:"uri"`
	Digest   string             `json:"digest"`
	Priority int                `json:"priority"`
	Labels   map[string]string  `json:"labels"`
	Links    catalogDetailLinks `json:"links"`
}

type provider struct {
	Name string  `json:"name"`
	URL  *string `json:"url,omitempty"`
}

type maintainer struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

type packageSummary struct {
	CatalogName        string              `json:"catalogName"`
	CatalogPriority    int                 `json:"catalogPriority"`
	Name               string              `json:"name"`
	DisplayName        string              `json:"displayName"`
	ShortDescription   string              `json:"shortDescription"`
	Provider           provider            `json:"provider"`
	IconAvailable      bool                `json:"iconAvailable"`
	DeprecationMessage string              `json:"deprecationMessage,omitempty"`
	Links              packageSummaryLinks `json:"links"`
}

type packageDetail struct {
	CatalogName        string             `json:"catalogName"`
	CatalogPriority    int                `json:"catalogPriority"`
	Name               string             `json:"name"`
	DisplayName        string             `json:"displayName"`
	ShortDescription   string             `json:"shortDescription"`
	Description        string             `json:"description"`
	Provider           provider           `json:"provider"`
	Maintainers        []maintainer       `json:"maintainers"`
	Keywords           []string           `json:"keywords"`
	SourceRepository   *string            `json:"sourceRepository,omitempty"`
	IconAvailable      bool               `json:"iconAvailable"`
	DeprecationMessage string             `json:"deprecationMessage,omitempty"`
	Links              packageDetailLinks `json:"links"`
}

type channelSummary struct {
	Name               string              `json:"name"`
	Path               []string            `json:"path"`
	DeprecationMessage string              `json:"deprecationMessage,omitempty"`
	Links              channelSummaryLinks `json:"links"`
}

type channelDetail struct {
	Name               string             `json:"name"`
	Path               []string           `json:"path"`
	DeprecationMessage string             `json:"deprecationMessage,omitempty"`
	Links              channelDetailLinks `json:"links"`
}

type bundleSummary struct {
	ID                 string             `json:"id"`
	PackageName        string             `json:"packageName"`
	Version            string             `json:"version"`
	Release            string             `json:"release"`
	URI                string             `json:"uri"`
	DeprecationMessage string             `json:"deprecationMessage,omitempty"`
	Links              bundleSummaryLinks `json:"links"`
}

type bundleDetail struct {
	ID                 string            `json:"id"`
	PackageName        string            `json:"packageName"`
	Version            string            `json:"version"`
	Release            string            `json:"release"`
	URI                string            `json:"uri"`
	MediaType          string            `json:"mediaType,omitempty"`
	ReleaseTimestamp   *time.Time        `json:"releaseTimestamp,omitempty"`
	DeprecationMessage string            `json:"deprecationMessage,omitempty"`
	Links              bundleDetailLinks `json:"links"`
}

type catalogCollection struct {
	Catalogs   []catalogSummary `json:"catalogs"`
	Total      int              `json:"total"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type packageCollection struct {
	Packages   []packageSummary `json:"packages"`
	Total      int              `json:"total"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type channelCollection struct {
	Channels   []channelSummary `json:"channels"`
	Total      int              `json:"total"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type bundleCollection struct {
	Bundles    []bundleSummary `json:"bundles"`
	Total      int             `json:"total"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type recommendation struct {
	Catalog    catalogSummary  `json:"catalog"`
	Package    packageSummary  `json:"package"`
	Candidates []bundleSummary `json:"candidates"`
	Total      int             `json:"total"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

func catalogSummaryFrom(prefix string, catalog catalogv1.Catalog) catalogSummary {
	self := catalogPath(catalog.Name())
	return catalogSummary{
		Name:     catalog.Name(),
		URI:      catalog.URI(),
		Digest:   catalog.Digest(),
		Priority: catalog.Priority(),
		Labels:   nonNilMap(catalog.Labels()),
		Links:    catalogSummaryLinks{Self: resourceLink{Href: prefix + self}},
	}
}

func catalogDetailFrom(prefix string, catalog catalogv1.Catalog) catalogDetail {
	self := catalogPath(catalog.Name())
	return catalogDetail{
		Name:     catalog.Name(),
		URI:      catalog.URI(),
		Digest:   catalog.Digest(),
		Priority: catalog.Priority(),
		Labels:   nonNilMap(catalog.Labels()),
		Links: catalogDetailLinks{
			Self: resourceLink{Href: prefix + self},
		},
	}
}

func packageSummaryFrom(prefix string, catalog catalogv1.Catalog, pkg catalogv1.UpdateGraph, metadata model.PackageMetadata) packageSummary {
	self := packagePath(catalog.Name(), pkg.Name())
	return packageSummary{
		CatalogName:        catalog.Name(),
		CatalogPriority:    catalog.Priority(),
		Name:               pkg.Name(),
		DisplayName:        metadata.DisplayName,
		ShortDescription:   metadata.ShortDescription,
		Provider:           providerFrom(metadata.Provider),
		IconAvailable:      metadata.IconAvailable,
		DeprecationMessage: deprecationMessage(pkg),
		Links: packageSummaryLinks{
			Self:    resourceLink{Href: prefix + self},
			Catalog: resourceLink{Href: prefix + catalogPath(catalog.Name())},
		},
	}
}

func packageDetailFrom(prefix string, catalog catalogv1.Catalog, pkg catalogv1.UpdateGraph, metadata model.PackageMetadata) packageDetail {
	self := packagePath(catalog.Name(), pkg.Name())
	maintainers := make([]maintainer, 0, len(metadata.Maintainers))
	for _, value := range metadata.Maintainers {
		maintainers = append(maintainers, maintainerFrom(value))
	}
	links := packageDetailLinks{
		Self:     resourceLink{Href: prefix + self},
		Catalog:  resourceLink{Href: prefix + catalogPath(catalog.Name())},
		Channels: resourceLink{Href: prefix + self + "/channels"},
		Bundles:  resourceLink{Href: prefix + self + "/bundles"},
	}
	if metadata.IconAvailable {
		links.Icon = &resourceLink{Href: prefix + self + "/icon"}
	}
	return packageDetail{
		CatalogName:        catalog.Name(),
		CatalogPriority:    catalog.Priority(),
		Name:               pkg.Name(),
		DisplayName:        metadata.DisplayName,
		ShortDescription:   metadata.ShortDescription,
		Description:        metadata.Description,
		Provider:           providerFrom(metadata.Provider),
		Maintainers:        maintainers,
		Keywords:           nonNilSlice(metadata.Keywords),
		SourceRepository:   stringFromURL(metadata.SourceRepository),
		IconAvailable:      metadata.IconAvailable,
		DeprecationMessage: deprecationMessage(pkg),
		Links:              links,
	}
}

func channelSummaryFrom(prefix, catalogName, packageName string, path []string, graph catalogv1.UpdateGraph) channelSummary {
	self := channelPath(catalogName, packageName, path)
	return channelSummary{
		Name:               graph.Name(),
		Path:               nonNilSlice(path),
		DeprecationMessage: deprecationMessage(graph),
		Links:              channelSummaryLinks{Self: resourceLink{Href: prefix + self}},
	}
}

func channelDetailFrom(prefix, catalogName, packageName string, path []string, graph catalogv1.UpdateGraph) channelDetail {
	self := channelPath(catalogName, packageName, path)
	return channelDetail{
		Name:               graph.Name(),
		Path:               nonNilSlice(path),
		DeprecationMessage: deprecationMessage(graph),
		Links: channelDetailLinks{
			Self:    resourceLink{Href: prefix + self},
			Package: resourceLink{Href: prefix + packagePath(catalogName, packageName)},
			Bundles: resourceLink{Href: prefix + self + "/bundles"},
		},
	}
}

func bundleSummaryFrom(prefix, catalogName, packageName string, bundle bundlev1.Bundle) bundleSummary {
	nvr := bundle.NameVersionRelease()
	return bundleSummary{
		ID:                 string(bundle.ID()),
		PackageName:        nvr.Name,
		Version:            nvr.Version.String(),
		Release:            nvr.Release.String(),
		URI:                bundle.URI(),
		DeprecationMessage: deprecationMessage(bundle),
		Links: bundleSummaryLinks{Self: resourceLink{
			Href: prefix + packagePath(catalogName, packageName) + "/bundles/" + escapePathPart(string(bundle.ID())),
		}},
	}
}

func bundleDetailFrom(ctx context.Context, prefix, catalogName, packageName string, bundle bundlev1.Bundle) (bundleDetail, error) {
	metadata, err := model.BundleMetadataFrom(ctx, bundle)
	if err != nil {
		return bundleDetail{}, err
	}
	nvr := bundle.NameVersionRelease()
	self := packagePath(catalogName, packageName) + "/bundles/" + escapePathPart(string(bundle.ID()))
	return bundleDetail{
		ID:                 string(bundle.ID()),
		PackageName:        nvr.Name,
		Version:            nvr.Version.String(),
		Release:            nvr.Release.String(),
		URI:                bundle.URI(),
		MediaType:          metadata.MediaType,
		ReleaseTimestamp:   metadata.ReleaseTimestamp,
		DeprecationMessage: deprecationMessage(bundle),
		Links: bundleDetailLinks{
			Self:    resourceLink{Href: prefix + self},
			Package: resourceLink{Href: prefix + packagePath(catalogName, packageName)},
			Catalog: resourceLink{Href: prefix + catalogPath(catalogName)},
		},
	}, nil
}

func providerFrom(value model.Provider) provider {
	return provider{Name: value.Name, URL: stringFromURL(value.URL)}
}

func maintainerFrom(value model.Maintainer) maintainer {
	result := maintainer{Name: value.Name}
	if value.Email != nil {
		email := value.Email.String()
		result.Email = &email
	}
	return result
}

func stringFromURL(value *model.URL) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
}

func deprecationMessage(value any) string {
	deprecated, ok := value.(catalogv1.Deprecated)
	if !ok {
		return ""
	}
	return deprecated.DeprecationMessage()
}

func catalogPath(catalogName string) string {
	return "/v1/catalogs/" + escapePathPart(catalogName)
}

func packagePath(catalogName, packageName string) string {
	return catalogPath(catalogName) + "/packages/" + escapePathPart(packageName)
}

func channelPath(catalogName, packageName string, path []string) string {
	encoded := make([]string, len(path))
	for i, segment := range path {
		encoded[i] = escapePathPart(segment)
	}
	return packagePath(catalogName, packageName) + "/channels/" + strings.Join(encoded, ":")
}

func escapePathPart(value string) string {
	if value == "." {
		return "%2E"
	}
	if value == ".." {
		return "%2E%2E"
	}
	return url.PathEscape(value)
}

func linkPrefix(r *http.Request) string {
	original, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		return ""
	}
	originalPath := original.EscapedPath()
	routedPath := r.URL.EscapedPath()
	if routedPath == "" || !strings.HasSuffix(originalPath, routedPath) {
		return ""
	}
	return strings.TrimSuffix(originalPath, routedPath)
}

func nonNilMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	return maps.Clone(value)
}

func nonNilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return append([]T(nil), value...)
}
