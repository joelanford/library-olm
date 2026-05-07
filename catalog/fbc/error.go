package fbc

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	iterxutil "github.com/joelanford/library-olm/internal/util/iterx"
)

type PackageError struct {
	Package string
	Errs    []error
}

func (e *PackageError) Error() string {
	msgs := make([]string, len(e.Errs))
	for i, err := range e.Errs {
		msgs[i] = err.Error()
	}
	return fmt.Sprintf("package %q: %d error(s): %s", e.Package, len(e.Errs), strings.Join(msgs, "; "))
}

func (e *PackageError) Unwrap() []error { return e.Errs }

type importError struct {
	err error
}

func (e *importError) Error() string   { return e.err.Error() }
func (e *importError) Unwrap() []error { return e.err.(interface{ Unwrap() []error }).Unwrap() }
func (e *importError) PartialImport()  {}

func mergePackageErrors(pkgErrMaps ...map[string][]error) error {
	merged := mergeMapSlices(pkgErrMaps...)
	pkgErrs := make([]error, 0, len(merged))
	for pkg, errs := range iterxutil.SortedMap(merged) {
		slices.SortFunc(errs, func(a, b error) int {
			return strings.Compare(a.Error(), b.Error())
		})
		pkgErrs = append(pkgErrs, &PackageError{Package: pkg, Errs: errs})
	}
	if joined := errors.Join(pkgErrs...); joined != nil {
		return &importError{err: joined}
	}
	return nil
}

func mergeMapSlices[K comparable, V any](ms ...map[K][]V) map[K][]V {
	merged := make(map[K][]V)
	for _, m := range ms {
		for k, vs := range m {
			merged[k] = append(merged[k], vs...)
		}
	}
	return merged
}
