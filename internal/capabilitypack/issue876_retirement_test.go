package capabilitypack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestIssue876RetiredSurfaceAndCapabilityAreRejected(t *testing.T) {
	retired := Surface("opencode")
	if slices.Contains(SupportedSurfaces(), retired) {
		t.Error("retired surface is advertised")
	}
	if err := validateV3Surfaces([]Surface{retired}); err == nil {
		t.Error("catalog accepts retired surface")
	}
	binding := Binding{Surface: SurfaceCodex, Name: "guide", Invocation: "$guide", Projection: "skill", Mode: "native", Sharing: "exclusive", Capabilities: []SurfaceCapability{{Type: "opencode-primary-prompt"}}}
	if err := validateBindingV3(Resource{Kind: "skill"}, binding, nil); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("retired capability: %v", err)
	}
}

func TestIssue876RetiredReceiptContractsFailClosed(t *testing.T) {
	retired := Surface("opencode")
	receipt := installedPackReceipt{Pack: installedPackIdentity{ID: "reviewed", Version: "1.0.0"}, Surface: retired}
	if _, err := activationDocumentFromReceipts(installedReceiptDocument{SchemaVersion: 1, Receipts: []installedPackReceipt{receipt}}); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
		t.Fatalf("global receipt: %v", err)
	}
	if err := validateProjectReceipts([]installedPackReceipt{receipt}); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
		t.Fatalf("project receipt: %v", err)
	}
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "state", "packs.json"))
	if _, err := store.SaveSnapshot(context.Background(), retired, 0, ActivationState{}); err == nil {
		t.Fatal("saved a retired surface")
	}
	if _, err := os.Stat(filepath.Dir(store.path)); !os.IsNotExist(err) {
		t.Fatalf("invalid save created state: %v", err)
	}
}

func TestIssue876RetiredPersonalStateBlocksProjectInstallAndApply(t *testing.T) {
	for _, afterPreview := range []bool{false, true} {
		t.Run(fmt.Sprint("after-preview=", afterPreview), func(t *testing.T) {
			project, packyHome := t.TempDir(), t.TempDir()
			pack := Pack{ID: "orphan-race", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, ReadinessObligations: []ReadinessObligation{}, Requires: Requirements{Tools: []string{}}, Resources: []Resource{}, Contract: Contract{OptionalModes: []OptionalMode{}}}
			adapter := &fakeSurfaceAdapter{}
			facade := NewFacade(Catalog{packs: []Pack{pack}})
			request := ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Selection: ResourceSelection{Mode: SelectionAll}}
			preview, err := facade.PreviewProjectInstall(context.Background(), request, adapter)
			if err != nil || preview.Disposition != ProjectInstallPreviewable {
				t.Fatalf("initial preview: %v %+v", err, preview)
			}
			// Model an old binary's valid receipt without asking the current writer to accept it.
			saveIssue797Orphan(t, packyHome, project, pack.ID, SurfaceCodex, nil)
			directory, err := projectActivationDirectory(packyHome, project)
			if err != nil {
				t.Fatal(err)
			}
			oldPath := filepath.Join(directory, projectActivationStateFile(pack.ID, SurfaceCodex))
			data, err := os.ReadFile(oldPath)
			if err != nil {
				t.Fatal(err)
			}
			data = []byte(strings.ReplaceAll(string(data), `"codex"`, `"opencode"`))
			retiredPath := filepath.Join(directory, projectActivationStateFile(pack.ID, Surface("opencode")))
			if err := os.WriteFile(retiredPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(oldPath); err != nil {
				t.Fatal(err)
			}
			if afterPreview {
				_, err = facade.ApplyProjectInstall(context.Background(), ProjectInstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: adapter})
			} else {
				_, err = facade.PreviewProjectInstall(context.Background(), request, adapter)
			}
			if err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
				t.Fatalf("retired personal receipt accepted: %v", err)
			}
			for _, file := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md"} {
				if _, err := os.Stat(filepath.Join(project, file)); !os.IsNotExist(err) {
					t.Fatalf("rejection wrote %s: %v", file, err)
				}
			}
			if got, err := os.ReadFile(retiredPath); err != nil || string(got) != string(data) {
				t.Fatalf("retired receipt changed: %v", err)
			}
		})
	}
}
