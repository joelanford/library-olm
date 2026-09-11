package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"slices"
	"strconv"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
)

func main() {
	s, err := sqlite.OpenStore(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	ch := newCatalogHandler(s)
	log.Fatal(http.ListenAndServe("localhost:8080", ch))
}

type ListPackagesRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

type ListPackagesResponse struct {
	Packages []PackageSummary `json:"packages"`
	Count    int              `json:"count"`
	Total    int              `json:"total"`
}

type catalogHandler struct {
	store catalogv1.StoreReader
	mux   *http.ServeMux
}

func newCatalogHandler(s catalogv1.StoreReader) *catalogHandler {
	h := &catalogHandler{store: s}
	mux := http.NewServeMux()
	mux.Handle("/v1/packages", http.HandlerFunc(h.listPackages))
	return &catalogHandler{store: s, mux: mux}
}

func (h *catalogHandler) listPackages(w http.ResponseWriter, r *http.Request) {
	req, err := parseListPackagesRequestFromURLQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	catalogs, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slices.SortFunc(catalogs, func(a, b catalogv1.Catalog) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	from := (req.Page - 1) * req.PageSize
	to := req.Page * req.PageSize
	results := make([]PackageSummary, 0, to-from)

	for _, cat := range catalogs {
		count := 0
		for ug, ugErr := range cat.ListPackages(r.Context()) {
			if ugErr != nil {
				http.Error(w, ugErr.Error(), http.StatusInternalServerError)
				return
			}
			if from <= count && count < to {
				results = append(results, PackageSummary{
					Name: ug.Name(),

				})
			}
			count++
		}
	}

}

func parseListPackagesRequestFromURLQuery(r *http.Request) (*ListPackagesRequest, error) {
	var req ListPackagesRequest
	pageStr, err := url.QueryUnescape(r.URL.Query().Get("page"))
	if err != nil {
		return nil, err
	}
	req.Page, err = strconv.Atoi(pageStr)
	if err != nil {
		return nil, err
	}
	pageSizeStr, err := url.QueryUnescape(r.URL.Query().Get("pageSize"))
	if err != nil {
		return nil, err
	}
	req.PageSize, err = strconv.Atoi(pageSizeStr)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (h *catalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

type PackageSummary struct {
	Name             string       `json:"name"`
	DisplayName      string       `json:"displayName"`
	ShortDescription string       `json:"shortDescription"`
	Provider         *Provider    `json:"provider"`
	Maintainers      []Maintainer `json:"maintainers"`
	Keywords         []string     `json:"keywords"`
	SourceRepository *URL         `json:"sourceRepository"`
	Deprecation      *Deprecation `json:"deprecation,omitempty"`
}

type PackageDetail struct {
	PackageSummary
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata"`
}

type Icon struct {
	Data      []byte
	MediaType string
}

type Provider struct {
	Name string `json:"name"`
	URL  *URL   `json:"url,omitempty"`
}

type Maintainer struct {
	Name  string        `json:"name"`
	Email *EmailAddress `json:"email,omitempty"`
}

type Deprecation struct {
	Message string `json:"message"`
}

type URL struct{ url.URL }

func ParseURL(value string) (*URL, error) {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%q is not an HTTP URL", value)
	}
	return &URL{URL: *parsed}, nil
}

func (u URL) MarshalJSON() ([]byte, error) { return json.Marshal(u.String()) }

func (u *URL) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseURL(value)
	if err != nil {
		return err
	}
	*u = *parsed
	return nil
}

type EmailAddress string

func (e EmailAddress) String() string { return string(e) }

func ParseEmailAddress(value string) (EmailAddress, error) {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return "", fmt.Errorf("%q is not a valid email address", value)
	}
	return EmailAddress(value), nil
}

func (e *EmailAddress) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseEmailAddress(value)
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}
