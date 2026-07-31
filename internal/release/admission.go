package release

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type AdmissionMode string

const (
	AdmissionFresh    AdmissionMode = "fresh"
	AdmissionDryRun   AdmissionMode = "dry-run"
	AdmissionRecovery AdmissionMode = "recovery"
)

// AdmissionObservation is a read-only projection of the event, refs, and
// release identity acquired by the workflow adapter.
type AdmissionObservation struct {
	EventName        string `json:"event_name"`
	EventRef         string `json:"event_ref"`
	RequestedMode    string `json:"requested_mode"`
	Repository       string `json:"repository"`
	Tag              string `json:"tag"`
	TagCommit        string `json:"tag_commit"`
	EventCommit      string `json:"event_commit"`
	CurrentMain      string `json:"current_main"`
	LatestVersion    string `json:"latest_version"`
	TagInMain        bool   `json:"tag_in_main"`
	ReleasePresent   bool   `json:"release_present"`
	ReleaseState     string `json:"release_state"`
	ReleaseTag       string `json:"release_tag"`
	ReleaseCommit    string `json:"release_commit"`
	ReleaseSealed    bool   `json:"release_sealed"`
	OriginalRunID    string `json:"original_run_id"`
	CandidateLocator string `json:"candidate_locator"`
}

type Admission struct {
	Mode                 AdmissionMode `json:"mode"`
	Tag                  string        `json:"tag"`
	ReleaseCommit        string        `json:"release_commit"`
	CurrentMain          string        `json:"current_main"`
	AttestationSourceRef string        `json:"attestation_source_ref"`
	ReleaseState         string        `json:"release_state"`
	OriginalRunID        string        `json:"original_run_id,omitempty"`
	CandidateLocator     string        `json:"candidate_locator,omitempty"`
}

func AdmitRelease(observed AdmissionObservation) (Admission, error) {
	if observed.Repository != PackyRepository {
		return Admission{}, errors.New("repository is not Packy's authorized repository")
	}
	if !versionPattern.MatchString(observed.Tag) {
		return Admission{}, errors.New("release tag must have form v0.x.y")
	}
	for name, commit := range map[string]string{
		"tag commit": observed.TagCommit, "current main": observed.CurrentMain,
	} {
		if !commitPattern.MatchString(commit) {
			return Admission{}, fmt.Errorf("%s must be one full lowercase 40-character SHA", name)
		}
	}
	mode, err := classifyAdmission(observed)
	if err != nil {
		return Admission{}, err
	}
	if mode != AdmissionRecovery && observed.LatestVersion != "" {
		if !versionPattern.MatchString(observed.LatestVersion) {
			return Admission{}, errors.New("latest version must have form v0.x.y")
		}
		newer, err := versionAfter(observed.Tag, observed.LatestVersion)
		if err != nil || !newer {
			return Admission{}, errors.New("release tag must be strictly newer than the latest version")
		}
	}

	if mode != AdmissionRecovery && observed.ReleasePresent {
		return Admission{}, errors.New("fresh publication and dry-run require the release to be absent")
	}
	if !observed.ReleasePresent && observed.ReleaseState != "absent" {
		return Admission{}, errors.New("absent release must have release state absent")
	}
	if mode == AdmissionRecovery {
		if !observed.ReleasePresent {
			return Admission{}, errors.New("recovery requires an existing release")
		}
		if observed.ReleaseState != "draft" && observed.ReleaseState != "published" {
			return Admission{}, errors.New("recovery release state must be draft or published")
		}
		if !observed.ReleaseSealed || observed.ReleaseTag != observed.Tag || observed.ReleaseCommit != observed.TagCommit {
			return Admission{}, errors.New("recovery release does not match the sealed tag and commit")
		}
	}

	result := Admission{
		Mode: mode, Tag: observed.Tag, ReleaseCommit: observed.TagCommit,
		CurrentMain: observed.CurrentMain, AttestationSourceRef: "refs/tags/" + observed.Tag,
		ReleaseState: observed.ReleaseState,
	}
	if mode == AdmissionRecovery {
		result.OriginalRunID = observed.OriginalRunID
		result.CandidateLocator = observed.CandidateLocator
	}
	return result, nil
}

func classifyAdmission(observed AdmissionObservation) (AdmissionMode, error) {
	switch observed.EventName {
	case "push":
		if observed.RequestedMode != "" || observed.EventRef != "refs/tags/"+observed.Tag {
			return "", errors.New("tag push must not request a manual mode and must match the exact tag ref")
		}
		if !commitPattern.MatchString(observed.EventCommit) ||
			observed.EventCommit != observed.TagCommit ||
			observed.TagCommit != observed.CurrentMain {
			return "", errors.New("fresh tag, event, and current main commits must be equal")
		}
		return AdmissionFresh, nil
	case "workflow_dispatch":
		if observed.EventRef != PackyMainRef || !observed.TagInMain {
			return "", errors.New("manual admission must run from current main for a tag in main history")
		}
		if observed.EventCommit != observed.CurrentMain {
			return "", errors.New("manual event commit must equal current main")
		}
		switch AdmissionMode(observed.RequestedMode) {
		case AdmissionDryRun, AdmissionRecovery:
			return AdmissionMode(observed.RequestedMode), nil
		default:
			return "", errors.New("manual admission mode must be dry-run or recovery")
		}
	default:
		return "", errors.New("release admission requires a tag push or manual workflow dispatch")
	}
}

func versionAfter(candidate, previous string) (bool, error) {
	parse := func(value string) ([2]uint64, error) {
		parts := strings.Split(strings.TrimPrefix(value, "v0."), ".")
		var result [2]uint64
		if len(parts) != 2 {
			return result, errors.New("invalid version")
		}
		for i := range parts {
			n, err := strconv.ParseUint(parts[i], 10, 64)
			if err != nil {
				return result, err
			}
			result[i] = n
		}
		return result, nil
	}
	next, err := parse(candidate)
	if err != nil {
		return false, err
	}
	last, err := parse(previous)
	if err != nil {
		return false, err
	}
	return next[0] > last[0] || next[0] == last[0] && next[1] > last[1], nil
}

type RefStateObservation struct {
	Tag               string `json:"tag"`
	ExpectedTagCommit string `json:"expected_tag_commit"`
	RemoteTagCommit   string `json:"remote_tag_commit"`
	ReleaseCommit     string `json:"release_commit"`
	CurrentMain       string `json:"current_main"`
	ReleaseInMain     bool   `json:"release_in_main"`
}

type VerifiedRefState struct {
	Verified      bool   `json:"verified"`
	Tag           string `json:"tag"`
	ReleaseCommit string `json:"release_commit"`
	CurrentMain   string `json:"current_main"`
}

// VerifyRefState rechecks immutable ref identity at a later privileged
// boundary while allowing protected main to advance.
func VerifyRefState(observed RefStateObservation) (VerifiedRefState, error) {
	if !versionPattern.MatchString(observed.Tag) {
		return VerifiedRefState{}, errors.New("release tag must have form v0.x.y")
	}
	for name, commit := range map[string]string{
		"expected tag commit": observed.ExpectedTagCommit,
		"remote tag commit":   observed.RemoteTagCommit,
		"release commit":      observed.ReleaseCommit,
		"current main":        observed.CurrentMain,
	} {
		if !commitPattern.MatchString(commit) {
			return VerifiedRefState{}, fmt.Errorf("%s must be one full lowercase 40-character SHA", name)
		}
	}
	if observed.RemoteTagCommit != observed.ExpectedTagCommit ||
		observed.ReleaseCommit != observed.ExpectedTagCommit {
		return VerifiedRefState{}, errors.New("remote tag no longer resolves to the sealed release commit")
	}
	if !observed.ReleaseInMain {
		return VerifiedRefState{}, errors.New("release commit is not in current main history")
	}
	return VerifiedRefState{
		Verified: true, Tag: observed.Tag, ReleaseCommit: observed.ReleaseCommit,
		CurrentMain: observed.CurrentMain,
	}, nil
}
