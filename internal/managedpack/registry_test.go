package managedpack

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReviewedRegistryContainsTheSevenApprovedManagedPackProjects(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join("..", "..", "managed-packs", "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Registration{
		{PackID: "addy", Project: "yersonargotev/skills-addy"},
		{PackID: "argote", Project: "yersonargotev/argote"},
		{PackID: "engram", Project: "yersonargotev/engram"},
		{PackID: "issue-delivery", Project: "yersonargotev/issue-deliver-pack"},
		{PackID: "matty", Project: "yersonargotev/skills-mattpocock"},
		{PackID: "orchestrate", Project: "yersonargotev/orchestrate-skill"},
		{PackID: "pstack", Project: "yersonargotev/pstack"},
	}
	if !reflect.DeepEqual(registry.Packs, want) {
		t.Fatalf("registry = %#v, want %#v", registry.Packs, want)
	}
}

func TestDecodeRegistryRejectsDuplicatePackAndProjectOwnership(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{"pack", `{"schema_version":1,"packs":[{"pack_id":"one","project":"owner/one"},{"pack_id":"one","project":"owner/two"}]}`, "sorted by pack_id without duplicates"},
		{"project", `{"schema_version":1,"packs":[{"pack_id":"one","project":"owner/shared"},{"pack_id":"two","project":"owner/shared"}]}`, "already owns"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeRegistry([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
