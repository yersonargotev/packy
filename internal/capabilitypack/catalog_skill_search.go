package capabilitypack

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// SkillSearchCandidate is the compact identity and on-demand compatibility of
// one current catalog skill; show --surface holds the complete metadata.
type SkillSearchCandidate struct {
	QualifiedSkill string `json:"qualified_skill"`
	Catalog        string `json:"catalog"`
	PackID         string `json:"pack_id"`
	PackVersion    string `json:"pack_version"`
	NativeName     string `json:"native_name"`
	Description    string `json:"description"`
	Compatible     bool   `json:"compatible"`
	Reason         string `json:"reason"`
}

type SkillSearchReport struct {
	Query   string                 `json:"query"`
	Surface Surface                `json:"surface"`
	Skills  []SkillSearchCandidate `json:"skills"`
}

// SkillSearch is a validated text query over one surface.
type SkillSearch struct {
	terms   []string
	surface Surface
}

func NewSkillSearch(query []string, surface Surface) (SkillSearch, error) {
	if err := validateDiscoverySurface(surface); err != nil {
		return SkillSearch{}, err
	}
	terms := strings.Fields(strings.Join(query, " "))
	if len(terms) == 0 {
		return SkillSearch{}, errors.New("search needs at least one query term; for example `packy search code review --surface agents`")
	}
	return SkillSearch{terms: terms, surface: surface}, nil
}

func (s SkillSearch) matches(candidate SkillDiscovery) bool {
	fields := []string{strings.ToLower(candidate.QualifiedSkill), strings.ToLower(candidate.NativeName), strings.ToLower(candidate.Description)}
	for _, term := range s.terms {
		term = strings.ToLower(term)
		if !slices.ContainsFunc(fields, func(field string) bool { return strings.Contains(field, term) }) {
			return false
		}
	}
	return true
}

// SearchSkills filters the skills of every current Pack, across all selected
// Catalog Snapshots, without ranking them.
func (c Catalog) SearchSkills(ctx context.Context, search SkillSearch) (SkillSearchReport, error) {
	packs, err := c.ListCurrent(ctx)
	if err != nil {
		return SkillSearchReport{}, err
	}
	report := SkillSearchReport{Query: strings.Join(search.terms, " "), Surface: search.surface, Skills: []SkillSearchCandidate{}}
	for _, pack := range packs {
		for _, candidate := range skillCandidates(packs, pack, search.surface) {
			if search.matches(candidate) {
				report.Skills = append(report.Skills, SkillSearchCandidate{QualifiedSkill: candidate.QualifiedSkill, Catalog: pack.CatalogRef.Repository, PackID: pack.ID, PackVersion: pack.Version, NativeName: candidate.NativeName, Description: candidate.Description, Compatible: candidate.Compatible, Reason: candidate.Reason})
			}
		}
	}
	slices.SortFunc(report.Skills, func(a, b SkillSearchCandidate) int {
		return strings.Compare(a.QualifiedSkill, b.QualifiedSkill)
	})
	return report, nil
}
