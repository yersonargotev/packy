package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type InstructionContribution struct{ ContributorID, Content string }

// UpsertInstructionContribution changes one contributor and retains every
// unrelated byte, including other Packy contributors.
func UpsertInstructionContribution(document string, contribution InstructionContribution) (string, error) {
	if strings.Count(document, instructionStart) != strings.Count(document, instructionEnd) || strings.Count(document, instructionStart) > 1 {
		return "", errors.New("invalid or duplicate Packy instruction markers")
	}
	start := "<!-- contributor:" + contribution.ContributorID + " -->"
	end := "<!-- /contributor:" + contribution.ContributorID + " -->"
	block := start + "\n" + strings.TrimSpace(contribution.Content) + "\n" + end
	if strings.Count(document, start) != strings.Count(document, end) || strings.Count(document, start) > 1 {
		return "", errors.New("invalid or duplicate Packy contributor markers")
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

func MergeInstructions(document string, contributions []InstructionContribution) (string, error) {
	if strings.Count(document, instructionStart) != strings.Count(document, instructionEnd) || strings.Count(document, instructionStart) > 1 {
		return "", errors.New("invalid or duplicate Packy instruction markers")
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
	return strings.TrimRight(document, "\n") + "\n\n" + block + "\n", nil
}

type CommandHookEntry struct {
	Type, Event, Matcher, Command string
	Args                          []string
	TimeoutSeconds                int
	Blocking                      bool
	Failure                       string
	Authorities                   []string
}

func (h CommandHookEntry) Validate() error {
	if h.Type != "command" || h.Event == "" || h.Command == "" || h.TimeoutSeconds <= 0 {
		return errors.New("noncanonical Claude command hook")
	}
	if h.Failure != "block" && h.Failure != "warn" {
		return errors.New("invalid Claude command hook failure behavior")
	}
	return nil
}
func (h CommandHookEntry) Fingerprint() string { return canonicalFingerprint(h) }

// MergeCommandHook preserves all unrelated JSON values and entries.
func MergeCommandHook(settings []byte, hook CommandHookEntry, remove bool) ([]byte, error) {
	if err := hook.Validate(); err != nil {
		return nil, err
	}
	var root map[string]any
	if len(strings.TrimSpace(string(settings))) == 0 {
		root = map[string]any{}
	} else if err := json.Unmarshal(settings, &root); err != nil {
		return nil, fmt.Errorf("invalid Claude settings JSON: %w", err)
	}
	hooks, ok := root["hooks"].(map[string]any)
	if root["hooks"] != nil && !ok {
		return nil, errors.New("Claude settings hooks must be an object")
	}
	if hooks == nil {
		hooks = map[string]any{}
	}
	entries, _ := hooks[hook.Event].([]any)
	wanted := hookJSON(hook)
	out := make([]any, 0, len(entries)+1)
	matches := 0
	for _, e := range entries {
		if canonicalFingerprint(e) == canonicalFingerprint(wanted) {
			matches++
			if remove {
				continue
			}
		}
		out = append(out, e)
	}
	if matches > 1 {
		return nil, errors.New("duplicate canonical Claude command hook")
	}
	if !remove && matches == 0 {
		out = append(out, wanted)
	}
	if len(out) == 0 {
		delete(hooks, hook.Event)
	} else {
		hooks[hook.Event] = out
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}
	return json.MarshalIndent(root, "", "  ")
}
func hookJSON(h CommandHookEntry) map[string]any {
	return map[string]any{"type": h.Type, "matcher": h.Matcher, "command": h.Command, "args": h.Args, "timeout_seconds": h.TimeoutSeconds, "blocking": h.Blocking, "failure": h.Failure, "authorities": h.Authorities}
}
