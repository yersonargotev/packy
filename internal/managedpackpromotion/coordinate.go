// Package managedpackpromotion owns Packy's private Managed Pack Promotion
// module.
package managedpackpromotion

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var packIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Coordinate identifies one exact Managed Pack release.
type Coordinate struct {
	PackID  string
	Version string
}

// ParseCoordinate parses the maintainer-facing <pack-id>@<version> form.
func ParseCoordinate(value string) (Coordinate, error) {
	if strings.Count(value, "@") != 1 {
		return Coordinate{}, fmt.Errorf("Managed Pack coordinate must be <pack-id>@<version>")
	}
	packID, version, _ := strings.Cut(value, "@")
	if !packIDPattern.MatchString(packID) {
		return Coordinate{}, fmt.Errorf("Managed Pack coordinate pack ID must be lowercase kebab-case")
	}
	if _, err := semver.StrictNewVersion(version); err != nil {
		return Coordinate{}, fmt.Errorf("Managed Pack coordinate version must be strict SemVer: %w", err)
	}
	return Coordinate{PackID: packID, Version: version}, nil
}

func (coordinate Coordinate) String() string {
	return coordinate.PackID + "@" + coordinate.Version
}
