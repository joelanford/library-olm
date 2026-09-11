package cataloghttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/labels"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

const (
	defaultLimit = 50
	maximumLimit = 200
	maximumBody  = 1 << 20
)

type collectionRequest struct {
	CatalogSelector string
	Cursor          string
	Limit           int
}

func (r collectionRequest) normalizedInput(resource string) any {
	return struct {
		Resource string `json:"resource"`
		Selector string `json:"selector,omitempty"`
	}{Resource: resource, Selector: r.CatalogSelector}
}

func parseCollectionRequest(r *http.Request, reader catalogv1.StoreReader, allowSelector bool) (collectionRequest, catalogv1.StoreReader, *problemDetails) {
	known := []string{"cursor", "limit"}
	if allowSelector {
		known = append(known, "catalogSelector")
	}
	query, problem := parseQuery(r.URL.RawQuery, known...)
	if problem != nil {
		return collectionRequest{}, nil, problem
	}

	request := collectionRequest{Limit: defaultLimit}
	if values, ok := query["limit"]; ok {
		limit, err := strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > maximumLimit {
			problem := newProblem(problemMalformedInput, invalidParam("limit", "must be an integer from 1 through 200"))
			return collectionRequest{}, nil, &problem
		}
		request.Limit = limit
	}
	if values, ok := query["cursor"]; ok {
		request.Cursor = values[0]
		if request.Cursor == "" {
			problem := newProblem(problemInvalidCursor, invalidParam("cursor", "must not be empty"))
			return collectionRequest{}, nil, &problem
		}
		if _, err := decodeCursor(request.Cursor); err != nil {
			problem := newProblem(problemInvalidCursor, invalidParam("cursor", "cannot be decoded"))
			return collectionRequest{}, nil, &problem
		}
	}

	selected := reader
	if values, ok := query["catalogSelector"]; ok {
		if values[0] == "" {
			return request, selected, nil
		}
		selector, err := labels.Parse(values[0])
		if err != nil {
			problem := newProblem(problemInvalidCatalogSelector, invalidParam("catalogSelector", "must use Kubernetes label selector syntax"))
			return collectionRequest{}, nil, &problem
		}
		request.CatalogSelector = selector.String()
		selected = reader.Select(selector)
	}
	return request, selected, nil
}

func parseQuery(rawQuery string, known ...string) (url.Values, *problemDetails) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		problem := newProblem(problemMalformedInput, invalidParam("query", "must be valid URL-encoded query data"))
		return nil, &problem
	}
	names := make([]string, 0, len(query))
	for name := range query {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		values := query[name]
		if !slices.Contains(known, name) {
			problem := newProblem(problemMalformedInput, invalidParam(name, "is not a recognized query parameter"))
			return nil, &problem
		}
		if len(values) != 1 {
			problem := newProblem(problemMalformedInput, invalidParam(name, "must be specified at most once"))
			return nil, &problem
		}
	}
	return query, nil
}

func rejectQuery(r *http.Request) *problemDetails {
	_, problem := parseQuery(r.URL.RawQuery)
	return problem
}

func decodeJSON(r *http.Request, destination any) *problemDetails {
	_, problem := decodeJSONBody(r, destination)
	return problem
}

func decodeJSONBody(r *http.Request, destination any) ([]byte, *problemDetails) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		problem := newProblem(problemUnsupportedMediaType)
		return nil, &problem
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maximumBody+1))
	if err != nil || len(body) > maximumBody {
		problem := newProblem(problemMalformedInput, invalidParam("body", "must be valid JSON no larger than 1 MiB"))
		return nil, &problem
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		problem := newProblem(problemMalformedInput, invalidParam("body", "must be a JSON object"))
		return nil, &problem
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		problem := newProblem(problemMalformedInput, invalidParam("body", jsonDecodeReason(err)))
		return nil, &problem
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		problem := newProblem(problemMalformedInput, invalidParam("body", "must contain exactly one JSON value"))
		return nil, &problem
	}
	return body, nil
}

func jsonDecodeReason(err error) string {
	if strings.Contains(err.Error(), "unknown field") {
		return "contains an unknown field"
	}
	if errors.Is(err, io.EOF) {
		return "must contain one JSON value"
	}
	return "must be valid JSON no larger than 1 MiB"
}

type upgradeConstraintPolicy string

const (
	policyCatalogProvided upgradeConstraintPolicy = "CatalogProvided"
	policySelfCertified   upgradeConstraintPolicy = "SelfCertified"
)

type bundleIdentity struct {
	ID          string `json:"id"`
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
	Release     string `json:"release"`
}

type recommendationRequest struct {
	CurrentBundle           *bundleIdentity         `json:"currentBundle,omitempty"`
	ChannelPaths            [][]string              `json:"channelPaths,omitempty"`
	VersionConstraint       string                  `json:"versionConstraint,omitempty"`
	UpgradeConstraintPolicy upgradeConstraintPolicy `json:"upgradeConstraintPolicy,omitempty"`
}

func decodeRecommendationRequest(r *http.Request, packageName string, request *recommendationRequest) *problemDetails {
	body, problem := decodeJSONBody(r, request)
	if problem != nil {
		return problem
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		problem := newProblem(problemMalformedInput, invalidParam("body", "must be a JSON object"))
		return &problem
	}
	for _, name := range []string{"currentBundle", "channelPaths", "versionConstraint", "upgradeConstraintPolicy"} {
		if raw, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			problem := newProblem(problemMalformedInput, invalidParam(name, "must not be null"))
			return &problem
		}
	}
	if raw, ok := fields["currentBundle"]; ok {
		var identityFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &identityFields); err != nil {
			problem := newProblem(problemMalformedInput, invalidParam("currentBundle", "must be a JSON object"))
			return &problem
		}
		for _, name := range []string{"id", "packageName", "version", "release"} {
			if value, ok := identityFields[name]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				problem := newProblem(problemMalformedInput, invalidParam("currentBundle."+name, "must not be null"))
				return &problem
			}
		}
	}
	if request.UpgradeConstraintPolicy == "" {
		request.UpgradeConstraintPolicy = policyCatalogProvided
	}
	if request.UpgradeConstraintPolicy != policyCatalogProvided && request.UpgradeConstraintPolicy != policySelfCertified {
		problem := newProblem(problemUnsupportedPolicy, invalidParam("upgradeConstraintPolicy", "must be CatalogProvided or SelfCertified"))
		return &problem
	}
	if request.CurrentBundle != nil {
		if problem := validateBundleIdentity(*request.CurrentBundle, packageName); problem != nil {
			return problem
		}
	}
	if request.ChannelPaths != nil {
		if len(request.ChannelPaths) == 0 {
			problem := newProblem(problemMalformedInput, invalidParam("channelPaths", "must contain at least one path"))
			return &problem
		}
		seen := map[string]struct{}{}
		for _, path := range request.ChannelPaths {
			if problem := validateChannelPath(path); problem != nil {
				return problem
			}
			key := strings.Join(path, "\x00")
			if _, ok := seen[key]; ok {
				problem := newProblem(problemMalformedInput, invalidParam("channelPaths", "must not contain duplicate paths"))
				return &problem
			}
			seen[key] = struct{}{}
		}
	}
	if request.VersionConstraint != "" {
		if _, err := mmsemver.NewConstraint(request.VersionConstraint); err != nil {
			problem := newProblem(problemInvalidVersionConstraint, invalidParam("versionConstraint", "must be a valid semantic-version constraint"))
			return &problem
		}
	}
	return nil
}

func validateBundleIdentity(identity bundleIdentity, packageName string) *problemDetails {
	for name, value := range map[string]string{
		"currentBundle.id":          identity.ID,
		"currentBundle.packageName": identity.PackageName,
		"currentBundle.version":     identity.Version,
	} {
		if value == "" {
			problem := newProblem(problemMalformedInput, invalidParam(name, "must not be empty"))
			return &problem
		}
	}
	if identity.PackageName != packageName {
		problem := newProblem(problemMalformedInput, invalidParam("currentBundle.packageName", "must equal the package path parameter"))
		return &problem
	}
	if _, err := bsemver.Parse(identity.Version); err != nil {
		problem := newProblem(problemMalformedInput, invalidParam("currentBundle.version", "must be a Semantic Versioning 2.0.0 value"))
		return &problem
	}
	if _, err := bundlev1.ParseRelease(identity.Release); err != nil {
		problem := newProblem(problemMalformedInput, invalidParam("currentBundle.release", "must be empty or a dot-separated release"))
		return &problem
	}
	return nil
}

func decodeChannelPath(encoded string) ([]string, *problemDetails) {
	path := strings.Split(encoded, ":")
	if problem := validateChannelPath(path); problem != nil {
		return nil, problem
	}
	return path, nil
}

func validateChannelPath(path []string) *problemDetails {
	if len(path) == 0 {
		problem := newProblem(problemMalformedInput, invalidParam("channelPath", "must contain at least one segment"))
		return &problem
	}
	for _, segment := range path {
		if segment == "" || strings.Contains(segment, ":") {
			problem := newProblem(problemMalformedInput, invalidParam("channelPath", "segments must be non-empty and must not contain colons"))
			return &problem
		}
	}
	return nil
}

func (r recommendationRequest) normalizedInput(packageName, selector string) any {
	return struct {
		Package  string                `json:"package"`
		Selector string                `json:"selector"`
		Request  recommendationRequest `json:"request"`
	}{Package: packageName, Selector: selector, Request: r}
}
