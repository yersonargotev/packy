package cli

import (
	"encoding/json"
	"io"
	"slices"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const packListJSONSchemaVersion = 1

type packListEntryJSON struct {
	ID          string                   `json:"id"`
	Version     string                   `json:"version"`
	Description string                   `json:"description"`
	Surfaces    []capabilitypack.Surface `json:"surfaces"`
}

type packListJSON struct {
	SchemaVersion int                 `json:"schema_version"`
	Report        string              `json:"report"`
	Packs         []packListEntryJSON `json:"packs"`
}

func packListDocument(packs []capabilitypack.Pack) packListJSON {
	ordered := append([]capabilitypack.Pack{}, packs...)
	slices.SortFunc(ordered, func(a, b capabilitypack.Pack) int {
		return strings.Compare(a.ID, b.ID)
	})
	entries := make([]packListEntryJSON, 0, len(ordered))
	for _, pack := range ordered {
		surfaces := append([]capabilitypack.Surface{}, pack.Surfaces...)
		slices.Sort(surfaces)
		entries = append(entries, packListEntryJSON{
			ID: pack.ID, Version: pack.Version, Description: pack.Description, Surfaces: surfaces,
		})
	}
	return packListJSON{SchemaVersion: packListJSONSchemaVersion, Report: "pack-list", Packs: entries}
}

func renderPackListJSON(w io.Writer, packs []capabilitypack.Pack) error {
	return json.NewEncoder(w).Encode(packListDocument(packs))
}
