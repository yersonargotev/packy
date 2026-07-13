// Package setuphealth owns read-only diagnosis of the base Matty setup.
package setuphealth

import (
	"fmt"
	"os"
	"strings"

	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/prompt"
)

type Severity string

const (
	Pass Severity = "PASS"
	Warn Severity = "WARN"
	Fail Severity = "FAIL"
)

type Check struct {
	Name     string
	Severity Severity
	Detail   string
}

type Summary struct {
	Status   string
	Passes   int
	Warnings int
	Failures int
}

type Context struct {
	HomeDir        string
	ConfigHome     string
	StateFile      string
	StateStatus    string
	AgentSkillsDir string
}

type Report struct {
	SchemaVersion int
	Kind          string
	Context       Context
	Checks        []Check
	Summary       Summary
}

// Config is the minimal resolved workstation context needed for base setup diagnosis.
type Config struct {
	HomeDir                string
	ConfigHome             string
	StateFile              string
	AgentSkillsDir         string
	SkillSourceRoot        string
	SkillSourceMissingHint string
	CodexPromptFile        string
	OpenCodeConfigFile     string
	OpenCodePromptFile     string
	PathEnv                string
	LocalBinEngram         string
	HomebrewPrefix         string
}

// ExecutableLookup is the least-authority active observation used by diagnosis.
type ExecutableLookup interface {
	LookPath(string) (string, error)
}

type Diagnoser struct {
	lookup ExecutableLookup
	facts  engrambin.Facts
}

func New(lookup ExecutableLookup, facts engrambin.Facts) Diagnoser {
	return Diagnoser{lookup: lookup, facts: facts.WithDefaults()}
}

// Diagnose observes the workstation once and returns the most complete read-only snapshot possible.
func (diagnoser Diagnoser) Diagnose(config Config) Report {
	state := corelifecycle.ObserveState(config.StateFile)
	checks := []Check{stateCheck(config, state)}
	checks = append(checks, skillChecks(config, state)...)
	checks = append(checks, engramChecks(diagnoser.lookup, config, state, diagnoser.facts)...)
	checks = append(checks, codexChecks(config)...)
	openCodeChecks, openCodeErr := openCodeChecks(config)
	if openCodeErr != nil {
		checks = append(checks, Check{Severity: Fail, Name: "opencode-config", Detail: openCodeErr.Error() + "; inspect the config or run matty install"})
	} else {
		checks = append(checks, openCodeChecks...)
	}
	summary := summarize(checks)
	stateStatus := "missing"
	if state.Found() {
		stateStatus = "present"
	}
	return Report{
		SchemaVersion: 1,
		Kind:          "doctor",
		Context: Context{
			HomeDir:        config.HomeDir,
			ConfigHome:     config.ConfigHome,
			StateFile:      config.StateFile,
			StateStatus:    stateStatus,
			AgentSkillsDir: config.AgentSkillsDir,
		},
		Checks:  checks,
		Summary: summary,
	}
}

func summarize(checks []Check) Summary {
	summary := Summary{Status: "healthy"}
	for _, check := range checks {
		switch check.Severity {
		case Pass:
			summary.Passes++
		case Warn:
			summary.Warnings++
		case Fail:
			summary.Failures++
		}
	}
	if summary.Failures > 0 {
		summary.Status = "failures"
	} else if summary.Warnings > 0 {
		summary.Status = "warnings"
	}
	return summary
}

func stateCheck(config Config, state corelifecycle.StateObservation) Check {
	if state.Condition() == corelifecycle.StateCorrupt {
		return Check{Severity: Fail, Name: "matty-state", Detail: state.Err().Error() + "; inspect or remove the corrupt state, then run matty install"}
	}
	if state.Condition() == corelifecycle.StateMissing {
		return Check{Severity: Warn, Name: "matty-state", Detail: "missing at " + config.StateFile + "; run matty install"}
	}
	if state.Condition() == corelifecycle.StateRecoveryRequired {
		return Check{Severity: Fail, Name: "matty-state", Detail: "classic installation was interrupted and requires recovery; run matty install or matty update to retry safely, or matty uninstall to remove only verified Matty-owned artifacts"}
	}
	return Check{Severity: Pass, Name: "matty-state", Detail: "present at " + config.StateFile}
}

func skillChecks(config Config, state corelifecycle.StateObservation) []Check {
	if !state.Found() {
		return []Check{{Severity: Warn, Name: "skill-symlinks", Detail: "state is missing, so Matty-owned skill links are unknown; run matty install"}}
	}
	managedSkills := state.Ownership().ManagedSkills
	if len(managedSkills) == 0 {
		return []Check{{Severity: Warn, Name: "skill-symlinks", Detail: zeroManagedSkillsDetail(config)}}
	}
	var missing, changed []string
	for _, link := range corelifecycle.ObserveManagedSkillLinks(managedSkills) {
		switch {
		case link.Err() != nil:
			changed = append(changed, fmt.Sprintf("%s (%v)", link.Name(), link.Err()))
		case link.Condition() == corelifecycle.SkillLinkMissing:
			missing = append(missing, link.Name())
		case link.Condition() == corelifecycle.SkillLinkUnmanagedPath:
			changed = append(changed, link.Name()+" is not a symlink")
		case link.Condition() == corelifecycle.SkillLinkUnmanagedSymlink:
			changed = append(changed, link.Name())
		case link.Condition() != corelifecycle.SkillLinkManaged:
			changed = append(changed, fmt.Sprintf("%s (unknown link status %s)", link.Name(), link.Condition()))
		}
	}
	if len(missing) == 0 && len(changed) == 0 {
		return []Check{{Severity: Pass, Name: "skill-symlinks", Detail: fmt.Sprintf("%d managed links under %s", len(managedSkills), config.AgentSkillsDir)}}
	}
	detail := "managed skill links need repair"
	if len(missing) > 0 {
		detail += "; missing: " + strings.Join(missing, ", ")
	}
	if len(changed) > 0 {
		detail += "; changed: " + strings.Join(changed, ", ")
	}
	return []Check{{Severity: Fail, Name: "skill-symlinks", Detail: detail + "; run matty update"}}
}

func zeroManagedSkillsDetail(config Config) string {
	detail := "state has no managed skills; run matty install"
	links, err := corelifecycle.ObserveExpectedManagedSkillLinks(corelifecycle.Config{
		AgentSkillsDir:         config.AgentSkillsDir,
		SkillSourceRoot:        config.SkillSourceRoot,
		SkillSourceMissingHint: config.SkillSourceMissingHint,
	})
	if err != nil {
		return detail + "; could not inspect expected skill links: " + err.Error()
	}
	var unmanaged []corelifecycle.SkillLinkObservation
	for _, link := range links {
		if link.Err() != nil {
			return detail + "; could not inspect expected skill links: " + link.Err().Error()
		}
		if link.Condition() == corelifecycle.SkillLinkUnmanagedSymlink {
			unmanaged = append(unmanaged, link)
		}
	}
	if len(links) == 0 || len(unmanaged)*2 <= len(links) {
		return detail
	}
	example := unmanaged[0]
	return fmt.Sprintf("state has no managed skills, but %d expected skill symlinks are unmanaged by current Matty state; setup may be incomplete. Example: %s -> %s. %s", len(unmanaged), example.LinkPath(), example.Target(), unmanagedSymlinkRecoveryAdvice())
}

func unmanagedSymlinkRecoveryAdvice() string {
	return "Safe recovery: verify these are stale Matty-created links, remove them, then run matty install; Matty will not overwrite arbitrary files or links."
}

func engramChecks(lookup ExecutableLookup, config Config, state corelifecycle.StateObservation, facts engrambin.Facts) []Check {
	checks := engramBinaryChecks(lookup, config, facts)
	canonical := engrambin.DiscoverHomebrew(config.HomebrewPrefix)
	checks = append(checks, engramRuntimeChecks(canonical, pathEngramExecutable(lookup, canonical), facts)...)
	if !state.Found() {
		return append(checks, Check{Severity: Warn, Name: "engram-setup", Detail: "state is missing, so delegated setup cannot be confirmed; run matty install"})
	}
	configuredSurfaces := state.ConfiguredSurfaces()
	if hasSurface(configuredSurfaces, "codex") && hasSurface(configuredSurfaces, "opencode") {
		return append(checks, Check{Severity: Pass, Name: "engram-setup", Detail: "state records Codex and OpenCode setup expectations; run matty update if Engram setup drifted"})
	}
	return append(checks, Check{Severity: Fail, Name: "engram-setup", Detail: "state does not record both Codex and OpenCode setup expectations; run matty update"})
}

func engramBinaryChecks(lookup ExecutableLookup, config Config, facts engrambin.Facts) []Check {
	canonical := engrambin.DiscoverHomebrewFromPrefixes(engrambin.HomebrewPrefixes(config.HomebrewPrefix))
	resolved, err := lookup.LookPath("engram")
	if err != nil {
		detail := "engram is not available on PATH; run matty install"
		if canonical != nil {
			detail = fmt.Sprintf("engram is not available on PATH; Homebrew Engram exists at %s; add it to PATH or run matty install", canonical.Path)
		}
		checks := []Check{{Severity: Fail, Name: "engram-binary", Detail: detail}}
		return append(checks, engramLocalBinChecks(config.LocalBinEngram, canonical)...)
	}
	prefixes := engrambin.HomebrewPrefixes(config.HomebrewPrefix)
	executablePaths := engrambin.UniquePaths(resolved, config.PathEnv, prefixes)
	executables := make([]engrambin.Executable, 0, len(executablePaths))
	for _, path := range executablePaths {
		version, versionErr := facts.Version(path)
		executables = append(executables, engrambin.NewExecutable(path, canonical, version, versionErr))
	}
	return engramDiagnosticChecks(executables, config.LocalBinEngram, canonical, prefixes)
}

func engramDiagnosticChecks(executables []engrambin.Executable, localBinEngram string, canonical *engrambin.Canonical, prefixes []string) []Check {
	pathEngram := executables[0]
	checks := []Check{engramPathCheck(pathEngram, canonical, prefixes)}
	for _, executable := range executables {
		if diagnosis := engrambin.DiagnoseVersion(executable); diagnosis != nil {
			checks = append(checks, Check{Severity: Warn, Name: "engram-version", Detail: diagnosis.Detail})
		}
	}
	checks = append(checks, engramVersionMismatchChecks(executables)...)
	if shadowing := engramHomebrewShadowingCheck(executables); shadowing != nil {
		checks = append(checks, *shadowing)
	}
	return append(checks, engramLocalBinChecks(localBinEngram, canonical)...)
}

func pathEngramExecutable(lookup ExecutableLookup, canonical *engrambin.Canonical) *engrambin.Executable {
	resolved, err := lookup.LookPath("engram")
	if err != nil {
		return nil
	}
	executable := engrambin.NewExecutable(resolved, canonical, "", nil)
	return &executable
}

func engramPathCheck(pathEngram engrambin.Executable, canonical *engrambin.Canonical, prefixes []string) Check {
	if pathEngram.Canonical {
		return Check{Severity: Pass, Name: "engram-binary", Detail: "PATH resolves to canonical Homebrew Engram: " + engrambin.Detail(pathEngram)}
	}
	expected := engrambin.ExpectedHomebrewPathFromPrefixes(prefixes)
	if canonical != nil {
		expected = canonical.Path
	}
	return Check{Severity: Warn, Name: "engram-binary", Detail: fmt.Sprintf("PATH resolves to non-Homebrew Engram %s; expected Homebrew-managed Engram at %s", engrambin.Detail(pathEngram), expected)}
}

func engramVersionMismatchChecks(executables []engrambin.Executable) []Check {
	versionByPath := []string{}
	versions := map[string]bool{}
	for _, executable := range executables {
		if executable.Version == "" {
			continue
		}
		versions[executable.Version] = true
		versionByPath = append(versionByPath, fmt.Sprintf("%s version %s", executable.Path, executable.Version))
	}
	if len(versions) <= 1 {
		return nil
	}
	return []Check{{Severity: Warn, Name: "engram-version-mismatch", Detail: "multiple engram executables report different versions: " + strings.Join(versionByPath, ", ")}}
}

func engramHomebrewShadowingCheck(executables []engrambin.Executable) *Check {
	if len(executables) < 2 || executables[0].Canonical {
		return nil
	}
	resolved := executables[0]
	for _, executable := range executables[1:] {
		if !executable.Canonical {
			continue
		}
		detail := fmt.Sprintf("%s appears before Homebrew Engram at %s", resolved.Path, executable.Path)
		if resolved.Version != "" {
			detail += " and reports version " + resolved.Version
		}
		if executable.Version != "" {
			detail += "; Homebrew reports version " + executable.Version
		}
		return &Check{Severity: Warn, Name: "engram-path-shadowing", Detail: detail}
	}
	return nil
}

func engramLocalBinChecks(localBinEngram string, canonical *engrambin.Canonical) []Check {
	diagnoses := engrambin.DiagnoseLocalBin(localBinEngram, canonical)
	checks := make([]Check, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
		severity := Warn
		if diagnosis.OK {
			severity = Pass
		}
		checks = append(checks, Check{Severity: severity, Name: "engram-local-bin", Detail: diagnosis.Detail})
	}
	return checks
}

func engramRuntimeChecks(canonical *engrambin.Canonical, pathEngram *engrambin.Executable, facts engrambin.Facts) []Check {
	processes, err := facts.ServeProcesses()
	if err != nil {
		return []Check{{Severity: Warn, Name: "engram-runtime", Detail: "could not inspect active engram serve processes: " + err.Error()}}
	}
	if len(processes) == 0 {
		return []Check{{Severity: Pass, Name: "engram-runtime", Detail: "no active engram serve process found"}}
	}
	checks := make([]Check, 0, len(processes))
	for _, process := range processes {
		diagnosis := engrambin.DiagnoseRuntimeProcess(process, canonical, pathEngram)
		detail := fmt.Sprintf("pid %d running %s", process.PID, process.ExecutablePath)
		if diagnosis.OK() {
			checks = append(checks, Check{Severity: Pass, Name: "engram-runtime", Detail: detail + " (matches PATH and canonical Homebrew Engram)"})
		} else {
			checks = append(checks, Check{Severity: Warn, Name: "engram-runtime", Detail: detail + "; " + strings.Join(diagnosis.Problems, "; ") + "; " + diagnosis.Remediation})
		}
	}
	return checks
}

func hasSurface(configuredSurfaces []string, want string) bool {
	for _, surface := range configuredSurfaces {
		if surface == want {
			return true
		}
	}
	return false
}

func codexChecks(config Config) []Check {
	data, err := os.ReadFile(config.CodexPromptFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Check{{Severity: Warn, Name: "codex-config", Detail: "missing Matty Codex prompt markers at " + config.CodexPromptFile + "; run matty install"}}
		}
		return []Check{{Severity: Fail, Name: "codex-config", Detail: fmt.Sprintf("cannot read %s: %v; inspect permissions", config.CodexPromptFile, err)}}
	}
	content := string(data)
	checks := []Check{}
	if strings.Contains(content, "<!-- matty:skills-router -->") && strings.Contains(content, "<!-- /matty:skills-router -->") {
		checks = append(checks, Check{Severity: Pass, Name: "codex-config", Detail: "Matty prompt markers are present"})
	} else {
		checks = append(checks, Check{Severity: Warn, Name: "codex-config", Detail: "Matty prompt markers are missing; run matty install"})
	}
	for _, warning := range prompt.DetectExternalManagedBlocks(content) {
		if strings.Contains(warning, "gentle-ai") {
			checks = append(checks, Check{Severity: Warn, Name: "codex-conflict", Detail: warning + "; inspect duplicate global instructions"})
		}
	}
	return checks
}

func openCodeChecks(config Config) ([]Check, error) {
	inspection, err := opencode.Inspect(config.OpenCodeConfigFile, config.OpenCodePromptFile)
	if err != nil {
		return nil, err
	}
	checks := []Check{}
	switch {
	case inspection.HasMattyInstruction && inspection.PromptExists:
		checks = append(checks, Check{Severity: Pass, Name: "opencode-config", Detail: "Matty instruction reference and prompt file are present"})
	case !inspection.ConfigExists:
		checks = append(checks, Check{Severity: Warn, Name: "opencode-config", Detail: "missing OpenCode config; run matty install"})
	case !inspection.HasMattyInstruction:
		checks = append(checks, Check{Severity: Warn, Name: "opencode-config", Detail: "Matty instruction reference is missing; run matty install"})
	default:
		checks = append(checks, Check{Severity: Warn, Name: "opencode-config", Detail: "Matty prompt file is missing; run matty update"})
	}
	for _, warning := range inspection.Warnings {
		checks = append(checks, Check{Severity: Warn, Name: "opencode-conflict", Detail: warning + "; inspect duplicate OpenCode overlays"})
	}
	return checks, nil
}
