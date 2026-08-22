package managedpackpromotion

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var gitObjectIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validateRelease(release Release, project string, coordinate Coordinate) string {
	version, err := semver.StrictNewVersion(coordinate.Version)
	if err != nil || version.Prerelease() != "" {
		return "promotion coordinate must identify a stable SemVer"
	}
	if !strings.EqualFold(release.Project, project) {
		return fmt.Sprintf("acquired project %q does not match registration %q", release.Project, project)
	}
	if !release.Public {
		return "Managed Pack Project is not public"
	}
	if !release.Published {
		return "release is not published"
	}
	if !release.Stable {
		return "release is not stable"
	}
	if release.Draft {
		return "release is a draft"
	}
	if release.Prerelease {
		return "release is a prerelease"
	}
	if !release.Immutable {
		return "release does not report GitHub immutability"
	}
	wantTag := "pack-v" + coordinate.Version
	if release.Tag != wantTag {
		return fmt.Sprintf("release tag %q does not match %q", release.Tag, wantTag)
	}
	if release.RepositoryID <= 0 || release.ReleaseID <= 0 {
		return "repository and release IDs must be positive"
	}
	if !gitObjectIDPattern.MatchString(release.TagRef.SHA) {
		return "tag ref SHA must be a full lowercase Git object ID"
	}
	if !gitObjectIDPattern.MatchString(release.CommitSHA) {
		return "commit SHA must be a full lowercase Git object ID"
	}
	if !gitObjectIDPattern.MatchString(release.RootTreeSHA) {
		return "root tree SHA must be a full lowercase Git object ID"
	}

	switch release.TagRef.Type {
	case GitObjectCommit:
		if len(release.TagObjects) != 0 {
			return "lightweight tag ref must not include annotated tag objects"
		}
		if release.TagRef.SHA != release.CommitSHA {
			return "lightweight tag ref does not resolve to the acquired commit"
		}
		return ""
	case GitObjectTag:
		return validateAnnotatedTagChain(release)
	default:
		return fmt.Sprintf("tag ref has unsupported object type %q", release.TagRef.Type)
	}
}

func validateAnnotatedTagChain(release Release) string {
	if len(release.TagObjects) == 0 {
		return "annotated tag ref has no tag object chain"
	}
	seen := make(map[string]bool, len(release.TagObjects))
	for i, object := range release.TagObjects {
		if !gitObjectIDPattern.MatchString(object.SHA) || !gitObjectIDPattern.MatchString(object.TargetSHA) {
			return "tag object chain contains an invalid Git object ID"
		}
		if seen[object.SHA] {
			return "tag object chain contains a cycle or duplicate"
		}
		seen[object.SHA] = true
		if i == 0 && release.TagRef.SHA != object.SHA {
			return "tag ref does not identify the first annotated tag object"
		}
		if i < len(release.TagObjects)-1 {
			next := release.TagObjects[i+1]
			if object.TargetType != GitObjectTag || object.TargetSHA != next.SHA {
				return "annotated tag object does not identify the next tag object"
			}
			continue
		}
		if object.TargetType != GitObjectCommit || object.TargetSHA != release.CommitSHA {
			return "peeled tag object does not identify the acquired commit"
		}
	}
	return ""
}
