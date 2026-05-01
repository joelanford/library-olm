package fbc

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/joelanford/library-olm/internal/util/iterx"
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

func mergePackageErrors(pkgErrMaps ...map[string][]error) error {
	merged := mergeMapSlices(pkgErrMaps...)
	pkgErrs := make([]error, 0, len(merged))
	for pkg, errs := range iterx.SortedMap(merged) {
		slices.SortFunc(errs, func(a, b error) int {
			return strings.Compare(a.Error(), b.Error())
		})
		pkgErrs = append(pkgErrs, &PackageError{Package: pkg, Errs: errs})
	}
	return errors.Join(pkgErrs...)
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
