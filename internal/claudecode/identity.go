package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
)

func Fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func canonicalFingerprint(v any) string { data, _ := json.Marshal(v); return Fingerprint(data) }

type SkillIdentity struct{ Surface, ProjectionID, Path, SymlinkType, ResolvedTarget, ExpectedSource, SourceTreeFingerprint string }

func (i SkillIdentity) Matches(surface, projectionID, path, expectedSource string, observation SkillObservation) bool {
	return i.Surface == surface && i.ProjectionID == projectionID && filepath.Clean(i.Path) == filepath.Clean(path) && i.SymlinkType == "directory" && observation.Kind == PathSymlink && i.ResolvedTarget == observation.ResolvedTarget && i.ExpectedSource == expectedSource && observation.ResolvedTarget == expectedSource && i.SourceTreeFingerprint == observation.TreeFingerprint
}

type InstructionIdentity struct{ Document, StartMarker, EndMarker, ContributorID, ContributionFingerprint string }
type AgentIdentity struct{ Surface, ProjectionID, Path, FileFingerprint string }

func Redact(value string, secrets ...string) string {
	for _, s := range secrets {
		if s != "" {
			value = strings.ReplaceAll(value, s, "[REDACTED]")
		}
	}
	return value
}

type OwnershipRecord struct {
	StateOwner, ContributorID, ID, Kind, Target, Fingerprint string
	Contributors                                             []string
	DeletionAuthorized                                       bool
	Skill                                                    SkillIdentity
	Composite                                                compositeOwnership
}

func (r OwnershipRecord) MatchesSkill(surface, projectionID, path, expectedSource string, observation SkillObservation) bool {
	return r.Fingerprint == observation.TreeFingerprint && r.Skill.Matches(surface, projectionID, path, expectedSource, observation)
}

// OwnershipSnapshot is a read-only composite view of records retained by their authoritative owners.
type OwnershipSnapshot struct {
	Records  []OwnershipRecord
	Revision string
}

type OwnershipSnapshotProvider interface {
	ObserveOwnership(context.Context) (OwnershipSnapshot, error)
}
type OwnershipSnapshotFunc func(context.Context) (OwnershipSnapshot, error)

func (f OwnershipSnapshotFunc) ObserveOwnership(ctx context.Context) (OwnershipSnapshot, error) {
	return f(ctx)
}
func StaticOwnershipSnapshot(snapshot OwnershipSnapshot) OwnershipSnapshotProvider {
	return OwnershipSnapshotFunc(func(context.Context) (OwnershipSnapshot, error) { return snapshot, nil })
}

func NewOwnershipSnapshot(records ...OwnershipRecord) OwnershipSnapshot {
	copyRecords := append([]OwnershipRecord(nil), records...)
	for i := range copyRecords {
		copyRecords[i].Contributors = append([]string(nil), copyRecords[i].Contributors...)
	}
	return OwnershipSnapshot{Records: copyRecords, Revision: canonicalFingerprint(copyRecords)}
}
