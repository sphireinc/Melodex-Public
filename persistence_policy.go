package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Versioned persistence has three intentionally different compatibility
// policies:
//
//   - authoritative app-owned files (settings, catalog, playlists, import
//     history, stage manifests, and metadata) reject unknown fields. Silently
//     dropping a field and writing the file back would destroy data from a
//     newer producer, so the user gets an actionable load error instead.
//   - disposable caches accept unknown fields and reset on an unsupported
//     version. Cache data can be rebuilt and must never block opening a
//     library or reuse an incompatible entry.
//   - portable library manifests are read-only import boundaries. They accept
//     additive unknown fields for forward compatibility, but reject an
//     unsupported schema version before applying the import.
//
// Version zero is the legacy/unversioned form only for formats whose migration
// function explicitly supports it. A non-zero version is never silently
// coerced to the current version; callers validate it before migration.

func decodeAuthoritativeJSON(data []byte, destination any, kind string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	return nil
}

func decodeCacheJSON(data []byte, destination any, kind string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	return nil
}

func decodePortableJSON(data []byte, destination any, kind string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode %s: %w", kind, err)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("trailing JSON value")
	}
	return fmt.Errorf("trailing JSON data: %w", err)
}

func validatePersistedSchemaVersion(kind string, version, current int) error {
	if version < 0 {
		return fmt.Errorf("invalid %s schema version %d", kind, version)
	}
	if version > current {
		return fmt.Errorf("unsupported %s schema version %d (maximum supported is %d)", kind, version, current)
	}
	return nil
}

func hasJSONField(data []byte, field string) (bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return false, err
	}
	_, ok := object[field]
	return ok, nil
}
