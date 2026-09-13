package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"net/url"
	"strings"
	"time"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

const (
	PackageMetadataProperty = "io.github.joelanford.library-olm.catalog-server.package-metadata"
	PackageIconProperty     = "io.github.joelanford.library-olm.catalog-server.package-icon"
	BundleMetadataProperty  = "io.github.joelanford.library-olm.catalog-server.bundle-metadata"
)

type PackageMetadata struct {
	DisplayName      string       `json:"displayName,omitempty"`
	ShortDescription string       `json:"shortDescription,omitempty"`
	Description      string       `json:"description,omitempty"`
	Provider         Provider     `json:"provider,omitempty"`
	Maintainers      []Maintainer `json:"maintainers,omitempty"`
	Keywords         []string     `json:"keywords,omitempty"`
	SourceRepository *URL         `json:"sourceRepository,omitempty"`
	IconAvailable    bool         `json:"iconAvailable,omitempty"`
}

type Provider struct {
	Name string `json:"name,omitempty"`
	URL  *URL   `json:"url,omitempty"`
}

type Maintainer struct {
	Name  string        `json:"name,omitempty"`
	Email *EmailAddress `json:"email,omitempty"`
}

type URL struct {
	value string
}

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

type EmailAddress struct {
	value string
}

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

type Icon struct {
	Content   []byte `json:"content"`
	MediaType string `json:"mediaType"`
}

type BundleMetadata struct {
	MediaType        string     `json:"mediaType,omitempty"`
	ReleaseTimestamp *time.Time `json:"releaseTimestamp,omitempty"`
}

func PackageMetadataFrom(ctx context.Context, graph catalogv1.UpdateGraph) (PackageMetadata, error) {
	var metadata PackageMetadata
	if err := decodeProperty(ctx, graph.Property, PackageMetadataProperty, &metadata); err != nil {
		return PackageMetadata{}, fmt.Errorf("decoding package metadata: %w", err)
	}
	return metadata, nil
}

func IconFrom(ctx context.Context, graph catalogv1.UpdateGraph) (io.ReadCloser, string, error) {
	var icon Icon
	found, err := decodeOptionalProperty(ctx, graph.Property, PackageIconProperty, &icon)
	if err != nil {
		return nil, "", fmt.Errorf("decoding package icon: %w", err)
	}
	if !found {
		return nil, "", nil
	}
	if err := ValidateIcon(icon); err != nil {
		return nil, "", err
	}
	return io.NopCloser(bytes.NewReader(icon.Content)), icon.MediaType, nil
}

func BundleMetadataFrom(ctx context.Context, bundle bundlev1.Bundle) (BundleMetadata, error) {
	var metadata BundleMetadata
	if err := decodeProperty(ctx, bundle.Property, BundleMetadataProperty, &metadata); err != nil {
		return BundleMetadata{}, fmt.Errorf("decoding bundle metadata: %w", err)
	}
	if err := ValidateBundleMetadata(metadata); err != nil {
		return BundleMetadata{}, err
	}
	return metadata, nil
}

func ValidateIcon(icon Icon) error {
	mediaType, _, err := mime.ParseMediaType(icon.MediaType)
	typeAndSubtype := strings.Split(mediaType, "/")
	if err != nil || len(typeAndSubtype) != 2 || typeAndSubtype[0] != "image" || typeAndSubtype[1] == "" || typeAndSubtype[1] == "*" {
		return fmt.Errorf("invalid icon media type %q", icon.MediaType)
	}
	return nil
}

func ValidateBundleMetadata(metadata BundleMetadata) error {
	if metadata.MediaType != "" {
		switch metadata.MediaType {
		case "registry+v1", "plain", "helm":
		default:
			return fmt.Errorf("invalid bundle media type %q", metadata.MediaType)
		}
	}
	return nil
}

func decodeProperty(ctx context.Context, get func(context.Context, string) (json.RawMessage, error), key string, dst any) error {
	_, err := decodeOptionalProperty(ctx, get, key, dst)
	return err
}

func decodeOptionalProperty(ctx context.Context, get func(context.Context, string) (json.RawMessage, error), key string, dst any) (bool, error) {
	raw, err := get(ctx, key)
	if err != nil {
		return false, err
	}
	if raw == nil {
		return false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, fmt.Errorf("property %q must not be null", key)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return false, err
	}
	return true, nil
}
