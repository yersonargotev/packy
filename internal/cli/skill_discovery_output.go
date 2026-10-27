package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func renderSkillDiscovery(w io.Writer, report capabilitypack.SkillDiscoveryReport, structured bool) error {
	if structured {
		return json.NewEncoder(w).Encode(struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			capabilitypack.SkillDiscoveryReport
		}{1, "skill-discovery", report})
	}
	if _, err := fmt.Fprintf(w, "%s %s — skill discovery on %s\n%s\n", report.PackID, report.PackVersion, report.Surface, report.Limitation); err != nil {
		return err
	}
	for _, skill := range report.Skills {
		if _, err := fmt.Fprintf(w, "Skill: %s (Pack %s) — %s\n%s\nNative name: %s; supported surfaces: %s\nOn-demand compatible: %s; reason: %s\nNative dependencies (Pack %s): %s; notices: %s\nKnown prerequisites (not verified): %s\n", skill.QualifiedSkill, skill.PackVersion, skill.Description, skill.Definition.Summary(), skill.NativeName, joinSurfaces(skill.SupportedSurfaces), yesNo(skill.Compatible), skill.Reason, skill.PackID, renderIdentityChain(skill.Dependencies), renderIdentityChain(skill.Notices), joinOrNoneReported(skill.Prerequisites)); err != nil {
			return err
		}
		for _, contract := range skill.PluginExports {
			if _, err := fmt.Fprintf(w, "Reviewed plugin export: %s; skill name: %s\n", contract.Format, contract.Name); err != nil {
				return err
			}
		}
		if skill.OnDemand != nil {
			if _, err := fmt.Fprintf(w, "Reviewed on-demand protocol: %s; entry document: %s\n", skill.OnDemand.Protocol, skill.OnDemand.AccessEntry()); err != nil {
				return err
			}
			for _, dependency := range skill.OnDemand.Dependencies {
				if _, err := fmt.Fprintf(w, "On-demand dependency: %s %s\n", dependency.Type, dependency.Target); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
