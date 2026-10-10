package catalogstore_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/catalogstore"
)

func TestLocalCandidateRejectsWholeIncompatibleCatalogWithoutOfficialSelection(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, want string
	}{
		{"valid", validManifest("candidate", "1.0.0"), ""},
		{"unknown vocabulary", strings.Replace(validManifest("candidate", "1.0.0"), `"resources":[`, `"future_field":true,"resources":[`, 1), "newer Packy engine"},
		{"unsupported capability", strings.Replace(validManifest("candidate", "1.0.0"), `"capabilities":[]`, `"capabilities":[{"type":"future-capability"}]`, 1), "newer Packy engine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := validRelease(t, strings.Repeat("a", 40), validManifest("candidate", "1.0.0"))
			replaceManifest(t, &release, tc.manifest)
			assets := t.TempDir()
			for _, asset := range release.Assets {
				if err := os.WriteFile(filepath.Join(assets, asset.Name), asset.Data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			root := t.TempDir()
			destination := filepath.Join(root, "candidate")
			snapshot, _, err := catalogstore.UnpackCandidate(context.Background(), assets, destination)
			if tc.want == "" {
				if err != nil || snapshot.Tag != "local-candidate" {
					t.Fatalf("local candidate: %#v %v", snapshot, err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("candidate rejection: %v; want %s", err, tc.want)
				}
				if _, err := os.Stat(destination); !os.IsNotExist(err) {
					t.Fatalf("invalid candidate left partial bytes: %v", err)
				}
			}
			if _, err := catalogstore.New(root, nil).Selected(); err == nil {
				t.Fatal("local candidate created official selection")
			}
		})
	}
}
