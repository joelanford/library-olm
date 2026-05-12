package internal

import (
	"context"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// NewPropertyWriter creates a PropertyWriter for a package backed by the given Writer.
func NewPropertyWriter(packageName string, w catalogv1.Writer) *propertyWriter {
	return &propertyWriter{packageName: packageName, w: w}
}

type propertyWriter struct {
	packageName string
	w           catalogv1.Writer
}

func (pw *propertyWriter) SetBundleProperty(_ context.Context, bundleName, key string, val any) error {
	return pw.w.SetBundleProperty(bundleName, key, val)
}

func (pw *propertyWriter) SetGraphProperty(_ context.Context, path []string, key string, val any) error {
	return pw.w.SetGraphProperty(append([]string{pw.packageName}, path...), key, val)
}
