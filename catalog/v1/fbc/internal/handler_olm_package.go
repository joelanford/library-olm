package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"time"

	bsemver "github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"go.podman.io/image/v5/docker/reference"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	internalproperty "github.com/joelanford/library-olm/catalog/v1/internal/property"
)

const (
	csvShortDescriptionAnnotation = "description"
	csvSourceRepositoryAnnotation = "repository"
	csvReleaseTimestampAnnotation = "createdAt"
	bundleMediaTypeAnnotation     = "operators.operatorframework.io.bundle.mediatype.v1"
)

type OLMPackageHandler struct{}

func (h *OLMPackageHandler) Schema() string { return declcfg.SchemaPackage }

type validatedBundle struct {
	name         string
	pkg          string
	version      string
	release      string
	uri          string
	versionValue bsemver.Version
	releaseValue bundlev1.Release
	hasCSV       bool
	csvMetadata  csvMetadata
	metadata     bundlev1.BundleMetadata
}

type csvMetadata struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Description string            `json:"description,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Keywords    []string          `json:"keywords,omitempty"`
	Maintainers []struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
	} `json:"maintainers,omitempty"`
	Provider struct {
		Name string `json:"name,omitempty"`
		URL  string `json:"url,omitempty"`
	} `json:"provider,omitempty"`
}

type packageSourceMetadata struct {
	description   string
	iconPresent   bool
	iconData      []byte
	iconMediaType string
}

type packageIcon struct {
	Content   []byte
	MediaType string
}

type channelEntry struct {
	name      string
	replaces  string
	skips     []string
	skipRange string
}

func (h *OLMPackageHandler) Normalize(ctx context.Context, rawDB *sql.DB, w catalogv1.Writer, packageName string) error {
	pkg := NewPackageAccessor(rawDB, packageName)

	bundles, err := h.validateBundles(pkg)
	if err != nil {
		return fmt.Errorf("validate bundles for %q: %w", packageName, err)
	}

	var channels []channelData
	for ch, err := range pkg.Channels() {
		if err != nil {
			return fmt.Errorf("read channels for %q: %w", packageName, err)
		}
		var entries []channelEntry
		for entry, err := range ch.Entries() {
			if err != nil {
				return fmt.Errorf("read channel entries for %q/%q: %w", packageName, ch.Name(), err)
			}
			entries = append(entries, channelEntry{
				name:      entry.BundleName(),
				replaces:  entry.Replaces(),
				skips:     entry.Skips(),
				skipRange: entry.SkipRange(),
			})
		}
		channels = append(channels, channelData{name: ch.Name(), entries: entries})
	}

	if err := h.validateEntries(packageName, bundles, channels); err != nil {
		return err
	}

	for _, ch := range channels {
		for _, e := range ch.entries {
			if e.skipRange != "" {
				if _, parseErr := bsemver.ParseRange(e.skipRange); parseErr != nil {
					return fmt.Errorf("channel %q: skipRange for %q: parse skipRange %q: %w", ch.name, e.name, e.skipRange, parseErr)
				}
			}
		}
	}

	metadata, icon, err := h.validatePackageMetadata(rawDB, packageName, bundles)
	if err != nil {
		return fmt.Errorf("validate metadata for %q: %w", packageName, err)
	}

	pkgPath := []string{packageName}
	if err := w.CreateGraph(pkgPath); err != nil {
		return fmt.Errorf("insert package graph %q: %w", packageName, err)
	}
	if err := w.SetGraphProperty(pkgPath, internalproperty.PackageMetadata, metadata); err != nil {
		return fmt.Errorf("set package metadata for %q: %w", packageName, err)
	}
	if icon != nil {
		if err := w.SetGraphProperty(pkgPath, internalproperty.PackageIcon, icon); err != nil {
			return fmt.Errorf("set package icon for %q: %w", packageName, err)
		}
	}

	for _, b := range bundles {
		if err := w.InsertBundle(b.name, b.pkg, b.version, b.release, b.uri); err != nil {
			return fmt.Errorf("insert bundle %q: %w", b.name, err)
		}
		if b.metadata.MediaType != "" || b.metadata.ReleaseTimestamp != nil {
			if err := w.SetBundleProperty(b.name, internalproperty.BundleMetadata, b.metadata); err != nil {
				return fmt.Errorf("set bundle metadata for %q: %w", b.name, err)
			}
		}
	}

	for _, ch := range channels {
		chPath := []string{packageName, ch.name}
		if err := w.CreateGraph(chPath); err != nil {
			return fmt.Errorf("insert graph for channel %q: %w", ch.name, err)
		}

		for _, e := range ch.entries {
			if err := w.AddBundleToGraph(chPath, e.name); err != nil {
				return fmt.Errorf("add bundle %q to channel %q: %w", e.name, ch.name, err)
			}
		}

		if err := h.writeChannelSuccessors(w, chPath, ch.entries); err != nil {
			return fmt.Errorf("compute successors for channel %q: %w", ch.name, err)
		}
	}

	if err := h.writeDeprecations(rawDB, w, packageName); err != nil {
		return fmt.Errorf("write deprecations for %q: %w", packageName, err)
	}

	return nil
}

func (h *OLMPackageHandler) validateBundles(pkg *PackageAccessor) ([]validatedBundle, error) {
	var bundles []validatedBundle
	for b, err := range pkg.Bundles() {
		if err != nil {
			return nil, err
		}
		version, err := bsemver.Parse(b.Version())
		if err != nil {
			return nil, fmt.Errorf("parse version %q for bundle %q: %w", b.Version(), b.Name(), err)
		}
		release := bundlev1.Release{}
		if b.Release() != "" {
			release, err = bundlev1.ParseRelease(b.Release())
			if err != nil {
				return nil, fmt.Errorf("parse release %q for bundle %q: %w", b.Release(), b.Name(), err)
			}
		}
		if b.Image() == "" {
			return nil, fmt.Errorf("bundle %q has no image", b.Name())
		}
		ref, err := reference.ParseNamed(b.Image())
		if err != nil {
			return nil, fmt.Errorf("parse image %q for bundle %q: %w", b.Image(), b.Name(), err)
		}
		if _, ok := ref.(reference.NamedTagged); !ok {
			if _, ok := ref.(reference.Canonical); !ok {
				return nil, fmt.Errorf("image %q for bundle %q must be tagged or canonical", b.Image(), b.Name())
			}
		}
		validated := validatedBundle{
			name:         b.Name(),
			pkg:          b.Package(),
			version:      b.Version(),
			release:      b.Release(),
			uri:          "docker://" + ref.String(),
			versionValue: version,
			releaseValue: release,
		}
		if raw := b.CSVMetadata(); len(raw) > 0 {
			validated.hasCSV = true
			if err := json.Unmarshal(raw, &validated.csvMetadata); err != nil {
				return nil, fmt.Errorf("parse olm.csv.metadata for bundle %q: %w", b.Name(), err)
			}
			if err := validateCSVMetadata(validated.csvMetadata); err != nil {
				return nil, fmt.Errorf("validate olm.csv.metadata for bundle %q: %w", b.Name(), err)
			}
			validated.metadata, err = bundleMetadataFromCSV(validated.csvMetadata)
			if err != nil {
				return nil, fmt.Errorf("validate bundle metadata for %q: %w", b.Name(), err)
			}
		}
		bundles = append(bundles, validated)
	}
	return bundles, nil
}

func (h *OLMPackageHandler) validatePackageMetadata(rawDB *sql.DB, packageName string, bundles []validatedBundle) (catalogv1.PackageMetadata, *packageIcon, error) {
	var source packageSourceMetadata
	if err := rawDB.QueryRow(
		"SELECT description, icon_present, icon_data, icon_media_type FROM "+TableRawPackage+" WHERE package_name = ?",
		packageName,
	).Scan(&source.description, &source.iconPresent, &source.iconData, &source.iconMediaType); err != nil {
		return catalogv1.PackageMetadata{}, nil, fmt.Errorf("read package source metadata: %w", err)
	}

	metadata := catalogv1.PackageMetadata{Description: source.description, IconAvailable: source.iconPresent}
	if selected := selectPresentationBundle(bundles); selected != nil {
		var err error
		metadata, err = packageMetadataFromCSV(metadata, selected.csvMetadata)
		if err != nil {
			return catalogv1.PackageMetadata{}, nil, err
		}
	}

	if !source.iconPresent {
		return metadata, nil, nil
	}
	mediaType, _, err := mime.ParseMediaType(source.iconMediaType)
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return catalogv1.PackageMetadata{}, nil, fmt.Errorf("invalid icon media type %q", source.iconMediaType)
	}
	return metadata, &packageIcon{Content: source.iconData, MediaType: source.iconMediaType}, nil
}

func selectPresentationBundle(bundles []validatedBundle) *validatedBundle {
	var selected *validatedBundle
	for i := range bundles {
		if !bundles[i].hasCSV {
			continue
		}
		if selected == nil || comparePresentationBundles(bundles[i], *selected) > 0 {
			selected = &bundles[i]
		}
	}
	return selected
}

func comparePresentationBundles(a, b validatedBundle) int {
	if c := a.versionValue.Compare(b.versionValue); c != 0 {
		return c
	}
	if c := a.releaseValue.Compare(b.releaseValue); c != 0 {
		return c
	}
	return strings.Compare(a.name, b.name)
}

func validateCSVMetadata(metadata csvMetadata) error {
	if metadata.Provider.URL != "" {
		if _, err := catalogv1.ParseURL(metadata.Provider.URL); err != nil {
			return fmt.Errorf("provider URL: %w", err)
		}
	}
	for i, maintainer := range metadata.Maintainers {
		if maintainer.Email == "" {
			continue
		}
		if _, err := catalogv1.ParseEmailAddress(maintainer.Email); err != nil {
			return fmt.Errorf("maintainer %d email: %w", i, err)
		}
	}
	if repository := metadata.Annotations[csvSourceRepositoryAnnotation]; repository != "" {
		if _, err := catalogv1.ParseURL(repository); err != nil {
			return fmt.Errorf("source repository: %w", err)
		}
	}
	return nil
}

func packageMetadataFromCSV(metadata catalogv1.PackageMetadata, csv csvMetadata) (catalogv1.PackageMetadata, error) {
	metadata.DisplayName = csv.DisplayName
	metadata.ShortDescription = csv.Annotations[csvShortDescriptionAnnotation]
	if metadata.Description == "" {
		metadata.Description = csv.Description
	}
	if metadata.Description == "" {
		metadata.Description = metadata.ShortDescription
	}
	metadata.Provider.Name = csv.Provider.Name
	metadata.Keywords = csv.Keywords
	for _, sourceMaintainer := range csv.Maintainers {
		maintainer := catalogv1.Maintainer{Name: sourceMaintainer.Name}
		if sourceMaintainer.Email != "" {
			email, err := catalogv1.ParseEmailAddress(sourceMaintainer.Email)
			if err != nil {
				return catalogv1.PackageMetadata{}, fmt.Errorf("maintainer email: %w", err)
			}
			maintainer.Email = &email
		}
		metadata.Maintainers = append(metadata.Maintainers, maintainer)
	}
	if csv.Provider.URL != "" {
		providerURL, err := catalogv1.ParseURL(csv.Provider.URL)
		if err != nil {
			return catalogv1.PackageMetadata{}, fmt.Errorf("provider URL: %w", err)
		}
		metadata.Provider.URL = &providerURL
	}
	if repository := csv.Annotations[csvSourceRepositoryAnnotation]; repository != "" {
		sourceURL, err := catalogv1.ParseURL(repository)
		if err != nil {
			return catalogv1.PackageMetadata{}, fmt.Errorf("source repository: %w", err)
		}
		metadata.SourceRepository = &sourceURL
	}
	return metadata, nil
}

func bundleMetadataFromCSV(csv csvMetadata) (bundlev1.BundleMetadata, error) {
	var metadata bundlev1.BundleMetadata
	if mediaType := csv.Annotations[bundleMediaTypeAnnotation]; mediaType != "" {
		switch mediaType {
		case "registry+v1", "plain", "helm":
			metadata.MediaType = mediaType
		default:
			return bundlev1.BundleMetadata{}, fmt.Errorf("invalid bundle media type %q", mediaType)
		}
	}
	if value := csv.Annotations[csvReleaseTimestampAnnotation]; value != "" {
		timestamp, err := parseReleaseTimestamp(value)
		if err != nil {
			return bundlev1.BundleMetadata{}, fmt.Errorf("release timestamp: %w", err)
		}
		metadata.ReleaseTimestamp = &timestamp
	}
	return metadata, nil
}

func parseReleaseTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", time.DateOnly} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
}

type channelData struct {
	name    string
	entries []channelEntry
}

func (h *OLMPackageHandler) validateEntries(packageName string, bundles []validatedBundle, channels []channelData) error {
	bundleNames := make(map[string]bool, len(bundles))
	for _, b := range bundles {
		bundleNames[b.name] = true
	}

	var missing []string
	for _, ch := range channels {
		for _, e := range ch.entries {
			if !bundleNames[e.name] {
				missing = append(missing, fmt.Sprintf("channel %q entry %q", ch.name, e.name))
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("validate package %q: channel entries reference unknown bundles: %s", packageName, strings.Join(missing, ", "))
	}
	return nil
}

func (h *OLMPackageHandler) writeDeprecations(rawDB *sql.DB, w catalogv1.Writer, packageName string) error {
	rows, err := rawDB.Query(
		"SELECT schema, name, message FROM "+TableRawDeprecationEntries+" WHERE package_name = ?",
		packageName,
	)
	if err != nil {
		return fmt.Errorf("querying deprecation entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var schema, name, message string
		if err := rows.Scan(&schema, &name, &message); err != nil {
			return err
		}
		switch schema {
		case declcfg.SchemaPackage:
			if err := w.SetGraphDeprecation([]string{packageName}, message); err != nil {
				return fmt.Errorf("set package deprecation: %w", err)
			}
		case declcfg.SchemaChannel:
			if err := w.SetGraphDeprecation([]string{packageName, name}, message); err != nil {
				return fmt.Errorf("set channel %q deprecation: %w", name, err)
			}
		case declcfg.SchemaBundle:
			if err := w.SetBundleDeprecation(name, message); err != nil {
				return fmt.Errorf("set bundle %q deprecation: %w", name, err)
			}
		}
	}
	return rows.Err()
}

func (h *OLMPackageHandler) writeChannelSuccessors(w catalogv1.Writer, chPath []string, entries []channelEntry) error {
	for _, e := range entries {
		if e.replaces != "" {
			if err := w.InsertBundle(e.replaces, "", "", "", ""); err != nil {
				return err
			}
			if err := w.AddEdge(chPath, e.replaces, e.name); err != nil {
				return err
			}
		}

		for _, skip := range e.skips {
			if err := w.InsertBundle(skip, "", "", "", ""); err != nil {
				return err
			}
			if err := w.AddEdge(chPath, skip, e.name); err != nil {
				return err
			}
		}

		if e.skipRange != "" {
			if err := w.AddPredecessorRange(chPath, e.name, e.skipRange); err != nil {
				return fmt.Errorf("skipRange for %q: %w", e.name, err)
			}
		}
	}
	return nil
}
