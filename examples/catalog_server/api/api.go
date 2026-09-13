// Package api exposes the example catalog server's HTTP API.
package api

import (
	"net/http"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	internalapi "github.com/joelanford/library-olm/examples/catalog_server/internal/api"
)

// NewHandler constructs a catalog API handler. The caller owns mounting,
// authentication, server configuration, and server lifecycle.
func NewHandler(reader catalogv1.StoreReader) http.Handler {
	return internalapi.NewHandler(reader)
}
