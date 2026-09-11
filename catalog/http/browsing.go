package cataloghttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

const (
	maximumChannelDepth = 1_000
	maximumChannelCount = 100_000
)

type channelCursorKey struct {
	Path string `json:"p"`
}

type bundleCursorKey struct {
	ID string `json:"i"`
}

func (h *handler) getPackage(w http.ResponseWriter, r *http.Request) {
	if problem := rejectQuery(r); problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	metadata, err := pkg.Metadata(r.Context())
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	writeJSON(w, r, packageDetailFrom(linkPrefix(r), catalog, pkg, metadata))
}

func (h *handler) listChannels(w http.ResponseWriter, r *http.Request) {
	request, _, problem := parseCollectionRequest(r, h.reader, false)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}

	channels, err := listDescendantChannels(r.Context(), linkPrefix(r), catalog.Name(), pkg.Name(), pkg)
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	sortChannelSummaries(channels)
	resource := packagePath(catalog.Name(), pkg.Name()) + "/channels"
	page, nextCursor, err := paginate(
		channels,
		request,
		resource,
		[]catalogv1.Catalog{catalog},
		func(channel channelSummary) channelCursorKey {
			return channelCursorKey{Path: strings.Join(channel.Path, ":")}
		},
		func(key channelCursorKey) bool { return key.Path != "" },
	)
	if err != nil {
		writeProblem(w, r, cursorProblem(err))
		return
	}
	writeJSON(w, r, channelCollection{Channels: page, Total: len(channels), NextCursor: nextCursor})
}

func (h *handler) getChannel(w http.ResponseWriter, r *http.Request) {
	if problem := rejectQuery(r); problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	path, problem := decodeChannelPath(r.PathValue("channelPath"))
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	graph, err := getGraph(r.Context(), pkg, path)
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	writeJSON(w, r, channelDetailFrom(linkPrefix(r), catalog.Name(), pkg.Name(), path, graph))
}

func (h *handler) listPackageBundles(w http.ResponseWriter, r *http.Request) {
	request, _, problem := parseCollectionRequest(r, h.reader, false)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	resource := packagePath(catalog.Name(), pkg.Name()) + "/bundles"
	h.writeBundleCollection(w, r, request, catalog, pkg.Name(), pkg, resource)
}

func (h *handler) listChannelBundles(w http.ResponseWriter, r *http.Request) {
	path, problem := decodeChannelPath(r.PathValue("channelPath"))
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	request, _, problem := parseCollectionRequest(r, h.reader, false)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	graph, err := getGraph(r.Context(), pkg, path)
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	resource := channelPath(catalog.Name(), pkg.Name(), path) + "/bundles"
	h.writeBundleCollection(w, r, request, catalog, pkg.Name(), graph, resource)
}

func (h *handler) writeBundleCollection(w http.ResponseWriter, r *http.Request, request collectionRequest, catalog catalogv1.Catalog, packageName string, graph catalogv1.UpdateGraph, resource string) {
	bundles, err := listBundles(r.Context(), graph)
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	sortBundles(bundles)
	page, nextCursor, err := paginate(
		bundles,
		request,
		resource,
		[]catalogv1.Catalog{catalog},
		func(bundle bundlev1.Bundle) bundleCursorKey {
			return bundleCursorKey{ID: string(bundle.ID())}
		},
		func(key bundleCursorKey) bool { return key.ID != "" },
	)
	if err != nil {
		writeProblem(w, r, cursorProblem(err))
		return
	}
	summaries := make([]bundleSummary, 0, len(page))
	for _, bundle := range page {
		summaries = append(summaries, bundleSummaryFrom(linkPrefix(r), catalog.Name(), packageName, bundle))
	}
	writeJSON(w, r, bundleCollection{Bundles: summaries, Total: len(bundles), NextCursor: nextCursor})
}

func (h *handler) getBundle(w http.ResponseWriter, r *http.Request) {
	if problem := rejectQuery(r); problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	catalog, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	bundles, err := listBundles(r.Context(), pkg)
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	var found bundlev1.Bundle
	for _, bundle := range bundles {
		if string(bundle.ID()) == r.PathValue("bundleID") {
			found = bundle
			break
		}
	}
	if found == nil {
		writeProblem(w, r, newProblem(problemNotFound))
		return
	}
	detail, err := bundleDetailFrom(r.Context(), linkPrefix(r), catalog.Name(), pkg.Name(), found)
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	writeJSON(w, r, detail)
}

func (h *handler) getIcon(w http.ResponseWriter, r *http.Request) {
	if problem := rejectQuery(r); problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	_, pkg, problem := h.getCatalogPackage(r)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	icon, err := pkg.Icon(r.Context())
	if err != nil {
		writeProblem(w, r, readProblem(err))
		return
	}
	if icon.Content == nil {
		writeProblem(w, r, newProblem(problemNotFound))
		return
	}
	defer func() { _ = icon.Close() }()
	if !validMediaType(icon.MediaType) {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}

	w.Header().Set("Content-Type", icon.MediaType)
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, icon.Content)
}

func (h *handler) getCatalogPackage(r *http.Request) (catalogv1.Catalog, catalogv1.Package, *problemDetails) {
	catalog, err := h.reader.Get(r.PathValue("catalog"))
	if err != nil {
		problem := readProblem(err)
		return nil, nil, &problem
	}
	pkg, err := catalog.GetPackage(r.Context(), r.PathValue("package"))
	if err != nil {
		problem := readProblem(err)
		return nil, nil, &problem
	}
	return catalog, pkg, nil
}

func readProblem(err error) problemDetails {
	if errors.Is(err, catalogv1.ErrNotFound) {
		return newProblem(problemNotFound)
	}
	return newProblem(problemCatalogReadFailure)
}

func listDescendantChannels(ctx context.Context, prefix, catalogName, packageName string, root catalogv1.UpdateGraph) ([]channelSummary, error) {
	channels := make([]channelSummary, 0)
	active := map[any]struct{}{}
	if comparableGraph(root) {
		active[root] = struct{}{}
	}
	if err := appendDescendantChannels(ctx, prefix, catalogName, packageName, root, nil, active, &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

func appendDescendantChannels(ctx context.Context, prefix, catalogName, packageName string, parent catalogv1.UpdateGraph, parentPath []string, active map[any]struct{}, channels *[]channelSummary) error {
	composite, ok := parent.(catalogv1.CompositeUpdateGraph)
	if !ok {
		return nil
	}
	if len(parentPath) >= maximumChannelDepth {
		return errors.New("channel graph nesting exceeds traversal limit")
	}
	for graph, err := range composite.ListGraphs(ctx) {
		if err != nil {
			return err
		}
		if graph == nil {
			return errors.New("channel graph is nil")
		}
		if len(*channels) >= maximumChannelCount {
			return errors.New("channel graph count exceeds traversal limit")
		}
		if graph.Name() == "" || strings.Contains(graph.Name(), ":") {
			return errors.New("channel graph has an invalid name")
		}
		tracked := comparableGraph(graph)
		if tracked {
			if _, cycle := active[graph]; cycle {
				return errors.New("channel graph contains a cycle")
			}
			active[graph] = struct{}{}
		}
		path := append(append([]string(nil), parentPath...), graph.Name())
		*channels = append(*channels, channelSummaryFrom(prefix, catalogName, packageName, path, graph))
		if err := appendDescendantChannels(ctx, prefix, catalogName, packageName, graph, path, active, channels); err != nil {
			return err
		}
		if tracked {
			delete(active, graph)
		}
	}
	return nil
}

func comparableGraph(graph catalogv1.UpdateGraph) bool {
	typeOf := reflect.TypeOf(graph)
	return typeOf != nil && typeOf.Comparable()
}

func getGraph(ctx context.Context, root catalogv1.UpdateGraph, path []string) (catalogv1.UpdateGraph, error) {
	graph := root
	for _, segment := range path {
		composite, ok := graph.(catalogv1.CompositeUpdateGraph)
		if !ok {
			return nil, fmt.Errorf("graph %q: %w", segment, catalogv1.ErrNotFound)
		}
		var err error
		graph, err = composite.GetGraph(ctx, segment)
		if err != nil {
			return nil, err
		}
		if graph == nil {
			return nil, errors.New("channel graph is nil")
		}
	}
	return graph, nil
}

func listBundles(ctx context.Context, graph catalogv1.UpdateGraph) ([]bundlev1.Bundle, error) {
	bundles := make([]bundlev1.Bundle, 0)
	for bundle, err := range graph.ListBundles(ctx) {
		if err != nil {
			return nil, err
		}
		if bundle == nil {
			return nil, errors.New("bundle is nil")
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func validMediaType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	typeAndSubtype := strings.Split(mediaType, "/")
	return len(typeAndSubtype) == 2 && typeAndSubtype[0] != "" && typeAndSubtype[1] != "" &&
		typeAndSubtype[0] != "*" && typeAndSubtype[1] != "*"
}
