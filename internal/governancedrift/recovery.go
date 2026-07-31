package governancedrift

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
)

var releaseTagPattern = regexp.MustCompile(`^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// RecoveryContract admits only the publication's own immutable latest-release
// transition. Every other governance expectation remains byte-for-byte exact.
func RecoveryContract(contract Contract, observation Observation, releaseTag string) (Contract, error) {
	if err := validateContract(contract); err != nil {
		return Contract{}, err
	}
	if err := validateObservation(observation); err != nil {
		return Contract{}, err
	}
	if !releaseTagPattern.MatchString(releaseTag) {
		return Contract{}, errors.New("recovery release tag must have form v0.x.y")
	}

	controlIndex := -1
	for i, control := range contract.Controls {
		if control.ID == "latest-release" {
			if controlIndex >= 0 {
				return Contract{}, errors.New("contract contains duplicate latest-release control")
			}
			controlIndex = i
		}
	}
	if controlIndex < 0 {
		return Contract{}, errors.New("contract omits latest-release control")
	}

	var actual SanitizedValue
	for _, control := range observation.Controls {
		if control.ID != "latest-release" {
			continue
		}
		if actual != "" {
			return Contract{}, errors.New("observation contains duplicate latest-release control")
		}
		if control.State != ObservationObserved {
			return Contract{}, errors.New("latest-release was not observed")
		}
		actual = control.Actual
	}
	if actual == "" {
		return Contract{}, errors.New("observation omits latest-release control")
	}

	var release struct {
		TagName     string `json:"tag_name"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		Immutable   bool   `json:"immutable"`
		PublishedAt string `json:"published_at"`
		Author      string `json:"author"`
		AssetCount  int    `json:"asset_count"`
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(actual)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&release); err != nil {
		return Contract{}, fmt.Errorf("decode latest-release observation: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Contract{}, errors.New("latest-release observation contains trailing data")
	}
	if release.TagName != releaseTag || release.Draft || release.Prerelease ||
		!release.Immutable || release.Author != "github-actions[bot]" || release.AssetCount != 7 {
		return Contract{}, errors.New("latest-release does not match the sealed immutable recovery shape")
	}
	if _, err := time.Parse(time.RFC3339, release.PublishedAt); err != nil {
		return Contract{}, fmt.Errorf("latest-release published_at: %w", err)
	}

	effective := Contract{
		SchemaVersion: contract.SchemaVersion,
		Controls:      append([]Control(nil), contract.Controls...),
	}
	effective.Controls[controlIndex].Boundaries = append([]Boundary(nil), contract.Controls[controlIndex].Boundaries...)
	effective.Controls[controlIndex].Expected = actual
	return effective, nil
}
