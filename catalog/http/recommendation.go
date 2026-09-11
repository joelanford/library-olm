package cataloghttp

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/labels"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	resolverv1 "github.com/joelanford/library-olm/resolver/v1"
)

type recommendationCursorKey struct {
	ID          string `json:"i"`
	PackageName string `json:"n"`
	Version     string `json:"v"`
	Release     string `json:"r"`
}

type currentBundleIdentity struct {
	id  bundlev1.BundleID
	nvr bundlev1.NameVersionRelease
}

type catalogSnapshotReader []catalogv1.Catalog

func (s catalogSnapshotReader) Get(name string) (catalogv1.Catalog, error) {
	for _, catalog := range s {
		if catalog.Name() == name {
			return catalog, nil
		}
	}
	return nil, fmt.Errorf("catalog %q: %w", name, catalogv1.ErrNotFound)
}

func (s catalogSnapshotReader) List() ([]catalogv1.Catalog, error) {
	return slices.Clone(s), nil
}

func (s catalogSnapshotReader) Select(selector labels.Selector) catalogv1.StoreReader {
	selected := make(catalogSnapshotReader, 0, len(s))
	for _, catalog := range s {
		if selector.Matches(labels.Set(catalog.Labels())) {
			selected = append(selected, catalog)
		}
	}
	return selected
}

func (i currentBundleIdentity) ID() bundlev1.BundleID {
	return i.id
}

func (i currentBundleIdentity) NameVersionRelease() bundlev1.NameVersionRelease {
	return i.nvr
}

func (h *handler) recommend(w http.ResponseWriter, r *http.Request) {
	collection, selected, problem := parseCollectionRequest(r, h.reader, true)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}

	packageName := r.PathValue("package")
	var request recommendationRequest
	if problem := decodeRecommendationRequest(r, packageName, &request); problem != nil {
		writeProblem(w, r, *problem)
		return
	}

	options, problem := resolveOptionsFrom(request)
	if problem != nil {
		writeProblem(w, r, *problem)
		return
	}
	normalizedInput := request.normalizedInput(packageName, collection.CatalogSelector)
	catalogs, err := selected.List()
	if err != nil {
		writeProblem(w, r, recommendationProblem(err))
		return
	}
	if collection.Cursor != "" {
		var key recommendationCursorKey
		if err := validateCursor(collection.Cursor, normalizedInput, catalogs, &key); err != nil {
			writeProblem(w, r, cursorProblem(err))
			return
		}
		if key.ID == "" || key.PackageName == "" || key.Version == "" {
			writeProblem(w, r, cursorProblem(errInvalidCursor))
			return
		}
	}

	result, err := resolverv1.Resolve(r.Context(), catalogSnapshotReader(catalogs), packageName, options...)
	if err != nil {
		writeProblem(w, r, recommendationProblem(err))
		return
	}
	if result == nil {
		writeProblem(w, r, newProblem(problemNotFound))
		return
	}
	if result.Catalog == nil || result.Package == nil {
		writeProblem(w, r, newProblem(problemCatalogReadFailure))
		return
	}

	metadata, err := result.Package.Metadata(r.Context())
	if err != nil {
		writeProblem(w, r, recommendationProblem(err))
		return
	}

	page, nextCursor, err := paginateWithInput(
		result.Bundles,
		collection.Cursor,
		collection.Limit,
		normalizedInput,
		catalogs,
		recommendationKey,
		func(key recommendationCursorKey) bool {
			return key.ID != "" && key.PackageName != "" && key.Version != ""
		},
	)
	if err != nil {
		writeProblem(w, r, cursorProblem(err))
		return
	}

	candidates := make([]bundleSummary, 0, len(page))
	for _, candidate := range page {
		candidates = append(candidates, bundleSummaryFrom(linkPrefix(r), result.Catalog.Name(), result.Package.Name(), candidate))
	}
	writeJSON(w, r, recommendation{
		Catalog:    catalogSummaryFrom(linkPrefix(r), result.Catalog),
		Package:    packageSummaryFrom(linkPrefix(r), result.Catalog, result.Package, metadata),
		Candidates: candidates,
		Total:      len(result.Bundles),
		NextCursor: nextCursor,
	})
}

func resolveOptionsFrom(request recommendationRequest) ([]resolverv1.ResolveOption, *problemDetails) {
	options := []resolverv1.ResolveOption{resolverv1.PreferNonDeprecatedBundles()}
	if request.ChannelPaths != nil {
		options = append(options, resolverv1.WithGraphs(request.ChannelPaths))
	}
	if request.VersionConstraint != "" {
		constraint, err := mmsemver.NewConstraint(request.VersionConstraint)
		if err != nil {
			problem := newProblem(problemInvalidVersionConstraint, invalidParam("versionConstraint", "must be a valid semantic-version constraint"))
			return nil, &problem
		}
		options = append(options, resolverv1.WithMastermindsVersionConstraint(*constraint))
	}
	if request.CurrentBundle != nil && request.UpgradeConstraintPolicy == policyCatalogProvided {
		identity, err := currentBundleIdentityFrom(*request.CurrentBundle)
		if err != nil {
			problem := newProblem(problemMalformedInput, invalidParam("currentBundle", "must be a valid bundle identity"))
			return nil, &problem
		}
		options = append(options, resolverv1.WithSuccessorsOf(identity))
	}
	return options, nil
}

func currentBundleIdentityFrom(identity bundleIdentity) (bundlev1.BundleIdentity, error) {
	version, err := bsemver.Parse(identity.Version)
	if err != nil {
		return nil, err
	}
	release, err := bundlev1.ParseRelease(identity.Release)
	if err != nil {
		return nil, err
	}
	return currentBundleIdentity{
		id: bundlev1.BundleID(identity.ID),
		nvr: bundlev1.NameVersionRelease{
			Name:    identity.PackageName,
			Version: version,
			Release: release,
		},
	}, nil
}

func recommendationKey(bundle bundlev1.Bundle) recommendationCursorKey {
	nvr := bundle.NameVersionRelease()
	return recommendationCursorKey{
		ID:          string(bundle.ID()),
		PackageName: nvr.Name,
		Version:     nvr.Version.String(),
		Release:     nvr.Release.String(),
	}
}

func recommendationProblem(err error) problemDetails {
	var ambiguity *resolverv1.AmbiguousPackageError
	if errors.As(err, &ambiguity) {
		return newProblem(problemAmbiguousPackage)
	}
	if errors.Is(err, catalogv1.ErrNotFound) {
		return newProblem(problemNotFound)
	}
	return newProblem(problemCatalogReadFailure)
}
