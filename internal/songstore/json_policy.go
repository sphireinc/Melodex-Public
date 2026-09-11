package songstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const songMetadataSchemaVersion = 1

// Song metadata is authoritative sidecar data. It is written by Melodex and
// may be rewritten after enrichment, so unknown fields are rejected instead
// of being silently discarded. Version zero is the legacy form; the current
// reader supplies the version after decoding it.
func decodeSongMetadataJSON(data []byte, metadata *SongMetadata) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(metadata); err != nil {
		return fmt.Errorf("decode song metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode song metadata: trailing JSON value")
		}
		return fmt.Errorf("decode song metadata: trailing JSON data: %w", err)
	}
	return nil
}

func validateSongMetadataVersion(version int) error {
	if version < 0 {
		return fmt.Errorf("invalid song metadata schema version %d", version)
	}
	if version > songMetadataSchemaVersion {
		return fmt.Errorf("unsupported song metadata schema version %d (maximum supported is %d)", version, songMetadataSchemaVersion)
	}
	return nil
}
