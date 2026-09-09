package opencode

import (
	"context"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"os"
	"path/filepath"
	"testing"
)

func TestReceiptInspectionPreservesAppliedEvidenceWithoutHistoricalSource(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "guide.md"), []byte("historical guidance\n"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), filepath.Join(root, "opencode.json"), filepath.Join(root, "prompt.md"))
	pack := capabilitypack.Pack{ID: "old", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	var actions []capabilitypack.ProjectionAction
	var owners []capabilitypack.ProjectionOwnership
	for _, p := range observed.Projections {
		actions = append(actions, p.Action)
		owners = append(owners, capabilitypack.ProjectionOwnership{ID: p.ID, Target: p.Action.Target, Fingerprint: p.DesiredFingerprint, PackID: "old", Surface: capabilitypack.SurfaceOpenCode})
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(bundle); err != nil {
		t.Fatal(err)
	}
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: owners})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != len(owners) || inspection.Readiness.AuthorizationObserved {
		t.Fatalf("receipt inspection = %#v", inspection)
	}
	for _, p := range inspection.Projections {
		if !p.Exists || p.Goal != capabilitypack.ProjectionPresent || p.DesiredFingerprint != p.ObservedFingerprint || p.Action.Content != "" || p.Action.Mode != "" {
			t.Fatalf("receipt projection = %#v", p)
		}
	}
	target := owners[0].Target
	if err := os.WriteFile(target, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	drifted, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: owners})
	if err != nil {
		t.Fatal(err)
	}
	if drifted.Projections[0].ObservedFingerprint == owners[0].Fingerprint {
		t.Fatal("drift was hidden")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	missing, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: owners})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Projections[0].Exists {
		t.Fatal("missing projection was hidden")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("inspection wrote target: %v", err)
	}
}

func TestReceiptInspectionRejectsInvalidOwnership(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "unavailable")
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), filepath.Join(root, "opencode.json"), filepath.Join(root, "prompt.md"))
	_, target, _, err := adapter.receiptTarget("instruction:guide")
	if err != nil {
		t.Fatal(err)
	}
	valid := capabilitypack.ProjectionOwnership{ID: "instruction:guide", Target: target, Fingerprint: "recorded", PackID: "old", Surface: capabilitypack.SurfaceOpenCode}
	for _, test := range []struct {
		name   string
		mutate func(*capabilitypack.ProjectionOwnership)
	}{
		{"unknown", func(o *capabilitypack.ProjectionOwnership) { o.ID = "unknown:guide" }},
		{"traversal", func(o *capabilitypack.ProjectionOwnership) { o.ID = "skill:../outside" }},
		{"target", func(o *capabilitypack.ProjectionOwnership) { o.Target = filepath.Join(root, "outside") }},
		{"surface", func(o *capabilitypack.ProjectionOwnership) { o.Surface = capabilitypack.SurfaceClaude }},
		{"fingerprint", func(o *capabilitypack.ProjectionOwnership) { o.Fingerprint = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := valid
			test.mutate(&owner)
			if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: []capabilitypack.ProjectionOwnership{owner}}); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: []capabilitypack.ProjectionOwnership{valid, valid}}); err == nil {
		t.Fatal("duplicate receipt accepted")
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: []capabilitypack.ProjectionOwnership{valid}}); err == nil {
		t.Fatal("symlink file accepted")
	}
}

func TestReceiptInspectionRejectsAmbiguousOpenCodeConfig(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "opencode.json")
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "skills"), config, filepath.Join(root, "prompt.md"))
	for _, content := range []string{`{"instructions":["guide.md","./guide.md"]}`, `{"instructions":["guide.md"],"instructions":["guide.md"]}`, `{"mcp":{"server":{},"server":{}}}`, `{invalid`} {
		if err := os.WriteFile(config, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		owner := capabilitypack.ProjectionOwnership{ID: "opencode-instruction-reference:guide", Target: config, Fingerprint: "recorded", PackID: "old", Surface: capabilitypack.SurfaceOpenCode}
		if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: []capabilitypack.ProjectionOwnership{owner}}); err == nil {
			t.Fatal("ambiguous config accepted")
		}
	}
}
