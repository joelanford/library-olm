package fbcextension

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bsemver "github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/v1/fbc"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
)

const (
	csvShortDescriptionAnnotation = "description"
	csvSourceRepositoryAnnotation = "repository"
	csvReleaseTimestampAnnotation = "createdAt"
	bundleMediaTypeAnnotation     = "operators.operatorframework.io.bundle.mediatype.v1"
)

type Extension struct{}

type packageData struct {
	Description string        `json:"description,omitempty"`
	Icon        *declcfg.Icon `json:"icon,omitempty"`
}

type bundleData struct {
	CSVMetadata json.RawMessage `json:"csvMetadata,omitempty"`
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

type presentationBundle struct {
	name     string
	version  bsemver.Version
	release  bundlev1.Release
	metadata csvMetadata
}

func New() *Extension { return &Extension{} }

func (e *Extension) OnPackage(pkg declcfg.Package) (any, error) {
	return packageData{Description: pkg.Description, Icon: pkg.Icon}, nil
}

func (e *Extension) OnChannel(channel declcfg.Channel) (any, error) {
	if strings.Contains(channel.Name, ":") {
		return nil, fmt.Errorf("channel name %q contains ':'", channel.Name)
	}
	return nil, nil
}

func (e *Extension) OnBundle(bundle declcfg.Bundle) (any, error) {
	var data bundleData
	for _, prop := range bundle.Properties {
		if prop.Type != property.TypeCSVMetadata {
			continue
		}
		if data.CSVMetadata != nil {
			return nil, fmt.Errorf("bundle %q has multiple %s properties", bundle.Name, property.TypeCSVMetadata)
		}
		if !json.Valid(prop.Value) {
			return nil, fmt.Errorf("parse %s property for bundle %q: invalid JSON", property.TypeCSVMetadata, bundle.Name)
		}
		data.CSVMetadata = prop.Value
	}
	return data, nil
}

func (e *Extension) OnDeprecation(declcfg.Deprecation) (any, error) { return nil, nil }

func (e *Extension) OnOther(declcfg.Meta) (any, error) { return nil, nil }

func (e *Extension) FinalizePackage(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
	var source packageData
	raw, err := pkg.ExtData()
	if err != nil {
		return fmt.Errorf("read package source metadata: %w", err)
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return fmt.Errorf("parse package source metadata: %w", err)
	}

	var selected *presentationBundle
	bundleMetadata := map[string]model.BundleMetadata{}
	for bundle, err := range pkg.Bundles() {
		if err != nil {
			return fmt.Errorf("read bundles: %w", err)
		}
		var data bundleData
		if err := json.Unmarshal(bundle.ExtData(), &data); err != nil {
			return fmt.Errorf("parse extension data for bundle %q: %w", bundle.Name(), err)
		}
		if data.CSVMetadata == nil {
			continue
		}
		var csv csvMetadata
		if err := json.Unmarshal(data.CSVMetadata, &csv); err != nil {
			return fmt.Errorf("parse olm.csv.metadata for bundle %q: %w", bundle.Name(), err)
		}
		if err := validateCSVMetadata(csv); err != nil {
			return fmt.Errorf("validate olm.csv.metadata for bundle %q: %w", bundle.Name(), err)
		}
		metadata, err := bundleMetadataFromCSV(csv)
		if err != nil {
			return fmt.Errorf("validate bundle metadata for %q: %w", bundle.Name(), err)
		}
		if metadata.MediaType != "" || metadata.ReleaseTimestamp != nil {
			bundleMetadata[bundle.Name()] = metadata
		}
		version, err := bsemver.Parse(bundle.Version())
		if err != nil {
			return fmt.Errorf("parse version %q for bundle %q: %w", bundle.Version(), bundle.Name(), err)
		}
		release, err := bundlev1.ParseRelease(bundle.Release())
		if err != nil {
			return fmt.Errorf("parse release %q for bundle %q: %w", bundle.Release(), bundle.Name(), err)
		}
		candidate := presentationBundle{name: bundle.Name(), version: version, release: release, metadata: csv}
		if selected == nil || comparePresentationBundles(candidate, *selected) > 0 {
			selected = &candidate
		}
	}

	metadata := model.PackageMetadata{Description: source.Description, IconAvailable: source.Icon != nil}
	if selected != nil {
		metadata, err = packageMetadataFromCSV(metadata, selected.metadata)
		if err != nil {
			return err
		}
	}
	var icon *model.Icon
	if source.Icon != nil {
		icon = &model.Icon{Content: source.Icon.Data, MediaType: source.Icon.MediaType}
		if err := model.ValidateIcon(*icon); err != nil {
			return err
		}
	}
	if err := w.SetGraphProperty(ctx, nil, model.PackageMetadataProperty, metadata); err != nil {
		return fmt.Errorf("set package metadata for %q: %w", pkg.Name(), err)
	}
	if icon != nil {
		if err := w.SetGraphProperty(ctx, nil, model.PackageIconProperty, icon); err != nil {
			return fmt.Errorf("set package icon for %q: %w", pkg.Name(), err)
		}
	}
	for bundleName, metadata := range bundleMetadata {
		if err := w.SetBundleProperty(ctx, bundleName, model.BundleMetadataProperty, metadata); err != nil {
			return fmt.Errorf("set bundle metadata for %q: %w", bundleName, err)
		}
	}
	return nil
}

func comparePresentationBundles(a, b presentationBundle) int {
	if comparison := a.version.Compare(b.version); comparison != 0 {
		return comparison
	}
	if comparison := a.release.Compare(b.release); comparison != 0 {
		return comparison
	}
	return strings.Compare(a.name, b.name)
}

func validateCSVMetadata(metadata csvMetadata) error {
	if metadata.Provider.URL != "" {
		if _, err := model.ParseURL(metadata.Provider.URL); err != nil {
			return fmt.Errorf("provider URL: %w", err)
		}
	}
	for index, maintainer := range metadata.Maintainers {
		if maintainer.Email == "" {
			continue
		}
		if _, err := model.ParseEmailAddress(maintainer.Email); err != nil {
			return fmt.Errorf("maintainer %d email: %w", index, err)
		}
	}
	if repository := metadata.Annotations[csvSourceRepositoryAnnotation]; repository != "" {
		if _, err := model.ParseURL(repository); err != nil {
			return fmt.Errorf("source repository: %w", err)
		}
	}
	return nil
}

func packageMetadataFromCSV(metadata model.PackageMetadata, csv csvMetadata) (model.PackageMetadata, error) {
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
		maintainer := model.Maintainer{Name: sourceMaintainer.Name}
		if sourceMaintainer.Email != "" {
			email, err := model.ParseEmailAddress(sourceMaintainer.Email)
			if err != nil {
				return model.PackageMetadata{}, fmt.Errorf("maintainer email: %w", err)
			}
			maintainer.Email = &email
		}
		metadata.Maintainers = append(metadata.Maintainers, maintainer)
	}
	if csv.Provider.URL != "" {
		providerURL, err := model.ParseURL(csv.Provider.URL)
		if err != nil {
			return model.PackageMetadata{}, fmt.Errorf("provider URL: %w", err)
		}
		metadata.Provider.URL = &providerURL
	}
	if repository := csv.Annotations[csvSourceRepositoryAnnotation]; repository != "" {
		sourceURL, err := model.ParseURL(repository)
		if err != nil {
			return model.PackageMetadata{}, fmt.Errorf("source repository: %w", err)
		}
		metadata.SourceRepository = &sourceURL
	}
	return metadata, nil
}

func bundleMetadataFromCSV(csv csvMetadata) (model.BundleMetadata, error) {
	metadata := model.BundleMetadata{MediaType: csv.Annotations[bundleMediaTypeAnnotation]}
	if value := csv.Annotations[csvReleaseTimestampAnnotation]; value != "" {
		timestamp, err := parseReleaseTimestamp(value)
		if err != nil {
			return model.BundleMetadata{}, fmt.Errorf("release timestamp: %w", err)
		}
		metadata.ReleaseTimestamp = &timestamp
	}
	if err := model.ValidateBundleMetadata(metadata); err != nil {
		return model.BundleMetadata{}, err
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

var _ fbc.OLMPackageExtension = (*Extension)(nil)
