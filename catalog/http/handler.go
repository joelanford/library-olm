package cataloghttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

type handler struct {
	reader catalogv1.StoreReader
}

// NewHandler constructs a catalog API handler. The caller owns mounting,
// authentication, server configuration, and server lifecycle.
func NewHandler(reader catalogv1.StoreReader) http.Handler {
	h := &handler{reader: reader}
	mux := http.NewServeMux()
	routes := []struct {
		method  string
		pattern string
		handler http.HandlerFunc
	}{
		{http.MethodGet, "/v1/catalogs", h.listCatalogs},
		{http.MethodGet, "/v1/catalogs/{catalog}", h.getCatalog},
		{http.MethodGet, "/v1/packages", h.listPackages},
		{http.MethodPost, "/v1/recommendations/{package}", h.recommend},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}", h.getPackage},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}/channels", h.listChannels},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}", h.getChannel},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}/bundles", h.listPackageBundles},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles", h.listChannelBundles},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}/bundles/{bundleID}", h.getBundle},
		{http.MethodGet, "/v1/catalogs/{catalog}/packages/{package}/icon", h.getIcon},
	}
	for _, route := range routes {
		mux.HandleFunc(route.method+" "+route.pattern, route.handler)
	}
	for _, route := range routes {
		fallback := methodNotAllowed(route.method)
		if route.method == http.MethodGet {
			mux.HandleFunc(http.MethodHead+" "+route.pattern, fallback)
		}
		mux.HandleFunc(route.pattern, fallback)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, newProblem(problemNotFound))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if escapedPath := r.URL.EscapedPath(); path.Clean(escapedPath) != escapedPath {
			writeProblem(w, r, newProblem(problemNotFound))
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func methodNotAllowed(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", method)
		writeProblem(w, r, newProblem(problemMethodNotAllowed))
	}
}

type catalogCursorKey struct {
	Name string `json:"n"`
}

type packageCursorKey struct {
	Name            string `json:"n"`
	CatalogPriority int    `json:"p"`
	CatalogName     string `json:"c"`
}

func (h *handler) listCatalogs(w http.ResponseWriter, r *http.Request) {
	request, selected, problem := parseCollectionRequest(r, h.reader, true)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalogs, err := selected.List()
	if err != nil {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}
	sortCatalogs(catalogs)

	page, nextCursor, err := paginate(
		catalogs,
		request,
		"/v1/catalogs",
		catalogs,
		func(catalog catalogv1.Catalog) catalogCursorKey {
			return catalogCursorKey{Name: catalog.Name()}
		},
		func(key catalogCursorKey) bool { return key.Name != "" },
	)
	if err != nil {
		writeProblem(w, r, cursorProblem(err))
		return
	}

	summaries := make([]catalogSummary, 0, len(page))
	for _, catalog := range page {
		summaries = append(summaries, catalogSummaryFrom(linkPrefix(r), catalog))
	}
	writeJSON(w, r, catalogCollection{Catalogs: summaries, Total: len(catalogs), NextCursor: nextCursor})
}

func (h *handler) getCatalog(w http.ResponseWriter, r *http.Request) {
	if problem := rejectQuery(r); problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, err := h.reader.Get(r.PathValue("catalog"))
	if errors.Is(err, catalogv1.ErrNotFound) {
		writeProblem(w, r, newProblem(problemNotFound))
		return
	}
	if err != nil {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}
	writeJSON(w, r, catalogDetailFrom(linkPrefix(r), catalog))
}

func (h *handler) listPackages(w http.ResponseWriter, r *http.Request) {
	request, selected, problem := parseCollectionRequest(r, h.reader, true)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalogs, err := selected.List()
	if err != nil {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}

	packages, err := packageSummaries(r.Context(), linkPrefix(r), catalogs)
	if err != nil {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}
	sortPackageSummaries(packages)

	page, nextCursor, err := paginate(
		packages,
		request,
		"/v1/packages",
		catalogs,
		func(pkg packageSummary) packageCursorKey {
			return packageCursorKey{Name: pkg.Name, CatalogPriority: pkg.CatalogPriority, CatalogName: pkg.CatalogName}
		},
		func(key packageCursorKey) bool { return key.Name != "" && key.CatalogName != "" },
	)
	if err != nil {
		writeProblem(w, r, cursorProblem(err))
		return
	}
	writeJSON(w, r, packageCollection{Packages: page, Total: len(packages), NextCursor: nextCursor})
}

func packageSummaries(ctx context.Context, prefix string, catalogs []catalogv1.Catalog) ([]packageSummary, error) {
	summaries := make([]packageSummary, 0)
	for _, catalog := range catalogs {
		for pkg, err := range catalog.ListPackages(ctx) {
			if err != nil {
				return nil, err
			}
			metadata, err := pkg.Metadata(ctx)
			if err != nil {
				return nil, err
			}
			summaries = append(summaries, packageSummaryFrom(prefix, catalog, pkg, metadata))
		}
	}
	return summaries, nil
}

func paginate[T any, K comparable](items []T, request collectionRequest, resource string, catalogs []catalogv1.Catalog, keyFor func(T) K, validKey func(K) bool) ([]T, string, error) {
	return paginateWithInput(items, request.Cursor, request.Limit, request.normalizedInput(resource), catalogs, keyFor, validKey)
}

func paginateWithInput[T any, K comparable](items []T, cursor string, limit int, normalizedInput any, catalogs []catalogv1.Catalog, keyFor func(T) K, validKey func(K) bool) ([]T, string, error) {
	start := 0
	if cursor != "" {
		var lastKey K
		if err := validateCursor(cursor, normalizedInput, catalogs, &lastKey); err != nil {
			return nil, "", err
		}
		if !validKey(lastKey) {
			return nil, "", errInvalidCursor
		}
		index := slices.IndexFunc(items, func(item T) bool { return keyFor(item) == lastKey })
		if index < 0 {
			return nil, "", errInvalidCursor
		}
		start = index + 1
	}

	end := min(start+limit, len(items))
	page := slices.Clone(items[start:end])
	if end == len(items) {
		return page, "", nil
	}
	nextCursor, err := encodeCursor(keyFor(items[end-1]), normalizedInput, catalogs)
	if err != nil {
		return nil, "", fmt.Errorf("encoding continuation cursor: %w", err)
	}
	return page, nextCursor, nil
}

func writeJSON(w http.ResponseWriter, r *http.Request, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
