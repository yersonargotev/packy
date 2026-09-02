package ci_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestIssue703EngramManagedPackOwnsCurrentRuntimeContractAndClosure(t *testing.T) {
	root := repositoryRoot(t)
	bundleRoot := filepath.Join(root, "bundle")
	manifestPath := filepath.Join(bundleRoot, "packs", "engram", "pack.json")
	pack, err := capabilitypack.LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "engram" || (pack.Version != "3.3.0" && pack.Version != "3.3.1") || !reflect.DeepEqual(pack.Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceCodex}) ||
		!reflect.DeepEqual(pack.ReadinessObligations, []capabilitypack.ReadinessObligation{capabilitypack.ReadinessRuntimeUsability, capabilitypack.ReadinessSurfaceAuthorization}) ||
		!reflect.DeepEqual(pack.Requires.Tools, []string{"engram"}) {
		t.Fatalf("Engram runtime identity = %#v", pack)
	}
	counts := pack.ResourceCounts()
	if counts.Skills != 1 || counts.Notices != 1 || counts.MCPServers != 0 || counts.Lifecycles != 0 || counts.Agents != 0 || counts.Commands != 0 || counts.Assets != 1 || counts.Instructions != 0 {
		t.Fatalf("Engram resource counts = %#v", counts)
	}
	if len(pack.Resources) != 3 {
		t.Fatalf("Engram resources = %#v", pack.Resources)
	}
	var asset, notice, skill *capabilitypack.Resource
	for i := range pack.Resources {
		resource := &pack.Resources[i]
		switch resource.Kind + ":" + resource.ID {
		case "asset:protocol-contract-v1":
			asset = resource
		case "notice:mit":
			notice = resource
		case "skill:engram-memory-cli":
			skill = resource
		default:
			t.Fatalf("unexpected Engram resource = %#v", resource)
		}
	}
	if asset == nil || asset.Source != "assets/protocol-contract-v1.json" || asset.Description != "Machine-verifiable Engram Protocol contract v1 compatibility metadata" || len(asset.Requires) != 0 || len(asset.Conflicts) != 0 || len(asset.Bindings) != 0 || len(asset.SurfaceExclusions) != 0 {
		t.Fatalf("Engram protocol contract asset = %#v", asset)
	}
	if notice == nil || notice.Source != "notices/engram-mit" || notice.License != "MIT" || notice.Attribution != "Copyright (c) 2026 Alan Buscaglia" {
		t.Fatalf("Engram legal notice = %#v", notice)
	}
	if skill == nil || skill.Source != "skills/engram-memory-cli" || !reflect.DeepEqual(skill.Notices, []string{"notice:mit"}) || len(skill.Bindings) != 1 {
		t.Fatalf("Engram skill = %#v", skill)
	}
	binding := skill.Bindings[0]
	if binding.Surface != capabilitypack.SurfaceCodex || binding.Projection != "skill" || binding.Name != "engram-memory-cli" || binding.Invocation != "$engram-memory-cli" || binding.Mode != "native" || binding.Sharing != "exclusive" || len(binding.Capabilities) != 1 || binding.Capabilities[0].Type != capabilitypack.SurfaceCapabilityExternalExecutableAcquisition || binding.Capabilities[0].ExternalExecutableAcquisition == nil || binding.Capabilities[0].ExternalExecutableAcquisition.Tool != "engram" {
		t.Fatalf("Engram skill binding = %#v", binding)
	}

	record, err := managedpack.LoadAdmissionRecord(filepath.Join(root, "managed-packs", "admissions", "engram", pack.Version+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if record.PackID != pack.ID || record.PackVersion != pack.Version || record.Project != "yersonargotev/engram" || !record.ReleaseImmutable || record.Tag != "pack-v"+pack.Version {
		t.Fatalf("Engram admission identity = %#v", record)
	}
	actual := currentManagedPackClosure(t, bundleRoot, manifestPath, pack.Resources)
	if !reflect.DeepEqual(actual, record.Files) {
		t.Fatalf("current Engram closure = %#v; want admitted closure %#v", actual, record.Files)
	}
	if _, err := os.Stat(filepath.Join(bundleRoot, "skills", "engram-memory")); !os.IsNotExist(err) {
		t.Fatalf("obsolete Packy-authored skill remains: %v", err)
	}
}

func currentManagedPackClosure(t *testing.T, bundleRoot, manifestPath string, resources []capabilitypack.Resource) []managedpack.FileRecord {
	t.Helper()
	records := map[string]managedpack.FileRecord{
		"pack.json": currentManagedPackFile(t, manifestPath, "pack.json"),
	}
	for _, resource := range resources {
		roots := []string{resource.Source}
		for _, binding := range resource.Bindings {
			roots = append(roots, binding.ReferencedSourcePaths()...)
		}
		for _, relativeRoot := range roots {
			if relativeRoot == "" {
				continue
			}
			err := filepath.WalkDir(filepath.Join(bundleRoot, filepath.FromSlash(relativeRoot)), func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil || entry.IsDir() {
					return walkErr
				}
				relative, err := filepath.Rel(bundleRoot, path)
				if err != nil {
					return err
				}
				relative = filepath.ToSlash(relative)
				records[relative] = currentManagedPackFile(t, path, relative)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	paths := make([]string, 0, len(records))
	for path := range records {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	closure := make([]managedpack.FileRecord, 0, len(paths))
	for _, path := range paths {
		closure = append(closure, records[path])
	}
	return closure
}

func currentManagedPackFile(t *testing.T, path, relative string) managedpack.FileRecord {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("current Managed Pack closure member %s has non-regular mode %s", relative, info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return managedpack.FileRecord{Path: relative, Mode: fmt.Sprintf("100%03o", info.Mode().Perm()), SHA256: hex.EncodeToString(digest[:])}
}
