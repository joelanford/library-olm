package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/fbc"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
	"github.com/joelanford/library-olm/examples/catalog_server/api"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/fbcextension"
)

//go:embed static
var staticFiles embed.FS

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args); err != nil {
		log.Printf("catalog server: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) (err error) {
	if len(args) != 2 || args[1] == "" {
		return errors.New("usage: catalog_server <fbc-dir-path>")
	}

	tmpDir, err := os.MkdirTemp("", "catalog-server")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		log.Printf("removing temporary catalog directory %s", tmpDir)
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("removing temp dir: %w", removeErr))
		}
	}()
	tmpDB := filepath.Join(tmpDir, "catalog.db")

	store, err := sqlite.OpenStore(tmpDB)
	if err != nil {
		return fmt.Errorf("opening catalog store: %w", err)
	}
	defer func() {
		log.Printf("closing catalog store")
		if closeErr := store.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing catalog store: %w", closeErr))
		}
		log.Printf("removing temporary catalog database %s", tmpDB)
		if removeErr := os.Remove(tmpDB); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("removing catalog database: %w", removeErr))
		}
	}()

	log.Printf("importing FBC catalog from %s", args[1])
	c, err := store.Set(ctx, "catalog",
		catalogv1.WithURI(args[1]),
		catalogv1.WithContent(fbc.NewFSImporter(os.DirFS(args[1]), fbc.WithOLMPackageExtension(fbcextension.New())), ""),
	)
	if err != nil {
		var partialImportErr catalogv1.PartialImportError
		if !errors.As(err, &partialImportErr) {
			return fmt.Errorf("importing catalog: %w", err)
		}
		log.Printf("catalog imported with errors: %v", partialImportErr)
	}
	log.Printf("imported catalog %q", c.Name())

	ui, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("loading embedded UI: %w", err)
	}
	hub, err := fs.Sub(staticFiles, "static/hub")
	if err != nil {
		return fmt.Errorf("loading embedded hub UI: %w", err)
	}
	apiHandler := api.NewHandler(store)
	mountedAPI := http.StripPrefix("/api", apiHandler)
	mux := http.NewServeMux()
	mux.Handle("/api/v1", mountedAPI)
	mux.Handle("/api/v1/", mountedAPI)
	mux.Handle("/ui/hub/", http.StripPrefix("/ui/hub", http.FileServer(http.FS(hub))))
	mux.Handle("/ui/workbench/", http.StripPrefix("/ui/workbench", http.FileServer(http.FS(ui))))

	server := &http.Server{
		Addr:              "localhost:8080",
		Handler:           requestLogger(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("serving catalog API at http://%s/api/v1, hub at http://%s/ui/hub/, and workbench at http://%s/ui/workbench/", server.Addr, server.Addr, server.Addr)
	if err := listenAndServe(ctx, server, time.Second*5); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving catalog API: %w", err)
	}
	return nil
}

type responseLogger struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *responseLogger) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseLogger) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (w *responseLogger) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logged := &responseLogger{ResponseWriter: w}
		next.ServeHTTP(logged, r)
		status := logged.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("%s %s status=%d bytes=%d duration=%s", r.Method, r.URL.RequestURI(), status, logged.bytes, time.Since(start))
	})
}

func listenAndServe(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancelOrderlyShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelOrderlyShutdown()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErr:
		return err
	}
}
