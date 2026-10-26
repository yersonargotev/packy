package claudecode

import (
	"fmt"
	"sort"
	"strings"
)

type InstructionContribution struct{ ContributorID, Content string }

// UpsertInstructionContribution changes one contributor and retains every
// unrelated byte, including other Packy contributors.
func UpsertInstructionContribution(document string, contribution InstructionContribution) (string, error) {
	if err := validateMarkerPair(document, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		return "", err
	}
	start := "<!-- contributor:" + contribution.ContributorID + " -->"
	end := "<!-- /contributor:" + contribution.ContributorID + " -->"
	block := start + "\n" + strings.TrimSpace(contribution.Content) + "\n" + end
	if err := validateMarkerPair(document, start, end, "Packy contributor"); err != nil {
		return "", err
	}
	if i := strings.Index(document, start); i >= 0 {
		j := strings.Index(document[i:], end)
		return document[:i] + block + document[i+j+len(end):], nil
	}
	if i := strings.Index(document, instructionStart); i >= 0 {
		j := i + strings.Index(document[i:], instructionEnd)
		prefix := document[:j]
		if !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		return prefix + block + "\n" + document[j:], nil
	}
	return MergeInstructions(document, []InstructionContribution{contribution})
}

func RemoveInstructionContribution(document, contributorID string) (string, error) {
	if err := validateMarkerPair(document, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		return "", err
	}
	start := "<!-- contributor:" + contributorID + " -->"
	end := "<!-- /contributor:" + contributorID + " -->"
	if err := validateMarkerPair(document, start, end, "Packy contributor"); err != nil {
		return "", err
	}
	i := strings.Index(document, start)
	if i < 0 {
		return document, nil
	}
	j := strings.Index(document[i:], end)
	result := document[:i] + document[i+j+len(end):]
	insideStart := strings.Index(result, instructionStart)
	insideEnd := strings.Index(result, instructionEnd)
	if insideStart >= 0 && insideEnd >= 0 && strings.TrimSpace(result[insideStart+len(instructionStart):insideEnd]) == "" {
		after := result[insideEnd+len(instructionEnd):]
		if insideStart == 0 && strings.HasPrefix(after, "\n") {
			after = after[1:]
		}
		result = result[:insideStart] + after
	}
	return result, nil
}

func MergeInstructions(document string, contributions []InstructionContribution) (string, error) {
	if err := validateMarkerPair(document, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	sort.Slice(contributions, func(i, j int) bool { return contributions[i].ContributorID < contributions[j].ContributorID })
	var body strings.Builder
	for _, c := range contributions {
		if strings.TrimSpace(c.ContributorID) == "" || seen[c.ContributorID] {
			return "", fmt.Errorf("duplicate or empty instruction contributor %q", c.ContributorID)
		}
		seen[c.ContributorID] = true
		fmt.Fprintf(&body, "<!-- contributor:%s -->\n%s\n<!-- /contributor:%s -->\n", c.ContributorID, strings.TrimSpace(c.Content), c.ContributorID)
	}
	block := instructionStart + "\n" + body.String() + instructionEnd
	if i := strings.Index(document, instructionStart); i >= 0 {
		j := strings.Index(document[i:], instructionEnd)
		return document[:i] + block + document[i+j+len(instructionEnd):], nil
	}
	if strings.TrimSpace(document) == "" {
		return block + "\n", nil
	}
	return block + "\n" + document, nil
}

func validateMarkerPair(document, start, end, label string) error {
	starts, ends := strings.Count(document, start), strings.Count(document, end)
	if starts != ends || starts > 1 {
		return fmt.Errorf("invalid or duplicate %s markers", label)
	}
	return nil
}
