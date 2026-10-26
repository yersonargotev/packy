package claudecode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

type PathKind string

const (
	PathMissing   PathKind = "missing"
	PathSymlink   PathKind = "symlink"
	PathFile      PathKind = "file"
	PathDirectory PathKind = "directory"
	PathOther     PathKind = "other"
)

type SkillObservation struct {
	Path                                                    string
	Kind                                                    PathKind
	Target, ResolvedTarget, ExpectedSource, TreeFingerprint string
	Err                                                     error
}
type InstructionObservation struct {
	Path, Revision          string
	MarkerCardinality       int
	Contributions           map[string]string
	ForeignContentPreserved bool
	Err                     error
}
type AgentObservation struct {
	Path        string
	Kind        PathKind
	Fingerprint string
	Err         error
}
type RuntimeEvidence struct{ Kind, ID, Signal, Revision string }
type OptionalAuthorityAvailability struct {
	ModeID, Authority string
	State             capabilitypack.OptionalAuthorityState
}
type AuthorizationObservation struct {
	PolicyObserved, ToolPermissionObserved bool
	Disabled, Shadowed                     bool
	OptionalAuthorities                    []OptionalAuthorityAvailability
	Err                                    error
}
type AuthorizationObserver interface {
	ObserveAuthorization(context.Context) AuthorizationObservation
}
type AuthorizationObserverFunc func(context.Context) AuthorizationObservation

func (f AuthorizationObserverFunc) ObserveAuthorization(ctx context.Context) AuthorizationObservation {
	return f(ctx)
}

func ObserveSkill(path, expectedSource string) SkillObservation {
	o := SkillObservation{Path: path, ExpectedSource: expectedSource}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		o.Kind = PathMissing
		return o
	}
	if err != nil {
		o.Err = err
		return o
	}
	if info.Mode()&os.ModeSymlink != 0 {
		o.Kind = PathSymlink
		o.Target, o.Err = os.Readlink(path)
		if o.Err == nil {
			o.ResolvedTarget, _ = filepath.EvalSymlinks(path)
			if o.ResolvedTarget != "" {
				o.TreeFingerprint, o.Err = localprojection.FingerprintTree(o.ResolvedTarget)
			}
		}
		return o
	}
	if info.IsDir() {
		o.Kind = PathDirectory
	} else if info.Mode().IsRegular() {
		o.Kind = PathFile
	} else {
		o.Kind = PathOther
	}
	return o
}
func ObserveAgent(path string) AgentObservation {
	fp, exists, err := localprojection.FingerprintPath(path)
	if err != nil {
		return AgentObservation{Path: path, Err: err}
	}
	if !exists {
		return AgentObservation{Path: path, Kind: PathMissing}
	}
	return AgentObservation{Path: path, Kind: PathFile, Fingerprint: fp}
}

const instructionStart = "<!-- packy:claude:start -->"
const instructionEnd = "<!-- packy:claude:end -->"

func ObserveInstructions(path string) InstructionObservation {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return InstructionObservation{Path: path, Revision: Fingerprint(nil), Contributions: map[string]string{}, ForeignContentPreserved: true}
	}
	if err != nil {
		return InstructionObservation{Path: path, Err: err}
	}
	s := string(b)
	starts, ends := strings.Count(s, instructionStart), strings.Count(s, instructionEnd)
	o := InstructionObservation{
		Path: path, Revision: Fingerprint(b), MarkerCardinality: starts,
		Contributions: map[string]string{}, ForeignContentPreserved: true,
	}
	if err := validateMarkerPair(s, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		o.Err = fmt.Errorf("%w: start=%d end=%d", err, starts, ends)
		return o
	}
	if starts == 0 {
		return o
	}
	inside := strings.SplitN(strings.SplitN(s, instructionStart, 2)[1], instructionEnd, 2)[0]
	for {
		i := strings.Index(inside, "<!-- contributor:")
		if i < 0 {
			break
		}
		markerEnd := strings.Index(inside[i:], " -->")
		if markerEnd < 0 {
			o.Err = errors.New("invalid contributor marker")
			return o
		}
		id := inside[i+len("<!-- contributor:") : i+markerEnd]
		startMarker := "<!-- contributor:" + id + " -->"
		endMarker := "<!-- /contributor:" + id + " -->"
		if _, duplicate := o.Contributions[id]; duplicate || strings.Count(inside, startMarker) != 1 || strings.Count(inside, endMarker) != 1 {
			o.Err = fmt.Errorf("duplicate or invalid contributor %q", id)
			return o
		}
		bodyStart := i + len(startMarker)
		bodyEndRel := strings.Index(inside[bodyStart:], endMarker)
		if bodyEndRel < 0 {
			o.Err = fmt.Errorf("missing end marker for contributor %q", id)
			return o
		}
		bodyEnd := bodyStart + bodyEndRel
		body := strings.TrimSpace(inside[bodyStart:bodyEnd])
		o.Contributions[id] = Fingerprint([]byte(body))
		inside = inside[bodyEnd+len(endMarker):]
	}
	return o
}
