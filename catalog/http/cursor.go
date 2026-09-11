package cataloghttp

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

const cursorHashBytes = sha256.Size

var (
	errInvalidCursor = errors.New("invalid cursor")
	errStaleCursor   = errors.New("stale cursor")
)

type cursor struct {
	inputHash    [sha256.Size]byte
	snapshotHash [sha256.Size]byte
	key          json.RawMessage
}

func encodeCursor(key, normalizedInput any, catalogs []catalogv1.Catalog) (string, error) {
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return "", fmt.Errorf("encoding cursor key: %w", err)
	}
	inputHash, err := hashJSON(normalizedInput)
	if err != nil {
		return "", fmt.Errorf("hashing normalized cursor input: %w", err)
	}
	snapshotHash, err := catalogSnapshotHash(catalogs)
	if err != nil {
		return "", err
	}
	payload := make([]byte, 0, 2*cursorHashBytes+len(keyJSON))
	payload = append(payload, inputHash[:]...)
	payload = append(payload, snapshotHash[:]...)
	payload = append(payload, keyJSON...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(token string) (cursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(payload) <= 2*cursorHashBytes {
		return cursor{}, errInvalidCursor
	}
	decoded := cursor{key: slices.Clone(payload[2*cursorHashBytes:])}
	copy(decoded.inputHash[:], payload[:cursorHashBytes])
	copy(decoded.snapshotHash[:], payload[cursorHashBytes:2*cursorHashBytes])
	if !json.Valid(decoded.key) {
		return cursor{}, errInvalidCursor
	}
	return decoded, nil
}

func validateCursor(token string, normalizedInput any, catalogs []catalogv1.Catalog, key any) error {
	decoded, err := decodeCursor(token)
	if err != nil {
		return err
	}
	inputHash, err := hashJSON(normalizedInput)
	if err != nil {
		return fmt.Errorf("hashing normalized cursor input: %w", err)
	}
	snapshotHash, err := catalogSnapshotHash(catalogs)
	if err != nil {
		return err
	}
	if !bytes.Equal(decoded.inputHash[:], inputHash[:]) || !bytes.Equal(decoded.snapshotHash[:], snapshotHash[:]) {
		return errStaleCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded.key))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(key); err != nil {
		return errInvalidCursor
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errInvalidCursor
	}
	return nil
}

func cursorProblem(err error) problemDetails {
	if errors.Is(err, errStaleCursor) {
		return newProblem(problemStaleCursor)
	}
	return newProblem(problemInvalidCursor, invalidParam("cursor", "cannot be decoded"))
}

func hashJSON(value any) ([sha256.Size]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

type snapshotCatalog struct {
	Name     string          `json:"n"`
	Labels   []snapshotLabel `json:"l"`
	Digest   string          `json:"d"`
	Priority int             `json:"p"`
}

type snapshotLabel struct {
	Name  string `json:"n"`
	Value string `json:"v"`
}

func catalogSnapshotHash(catalogs []catalogv1.Catalog) ([sha256.Size]byte, error) {
	snapshot := make([]snapshotCatalog, 0, len(catalogs))
	for _, catalog := range catalogs {
		labels := catalog.Labels()
		labelNames := make([]string, 0, len(labels))
		for name := range labels {
			labelNames = append(labelNames, name)
		}
		sort.Strings(labelNames)
		canonicalLabels := make([]snapshotLabel, 0, len(labelNames))
		for _, name := range labelNames {
			canonicalLabels = append(canonicalLabels, snapshotLabel{Name: name, Value: labels[name]})
		}
		snapshot = append(snapshot, snapshotCatalog{
			Name:     catalog.Name(),
			Labels:   canonicalLabels,
			Digest:   catalog.Digest(),
			Priority: catalog.Priority(),
		})
	}
	slices.SortFunc(snapshot, func(a, b snapshotCatalog) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	hash, err := hashJSON(snapshot)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("hashing catalog snapshot: %w", err)
	}
	return hash, nil
}
