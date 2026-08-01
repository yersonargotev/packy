// Package setuphealth owns read-only diagnosis of Packy core.
package setuphealth

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
	HomeDir    string
	ConfigHome string
}

type Report struct {
	SchemaVersion int
	Kind          string
	Context       Context
	Checks        []Check
	Summary       Summary
}

// Diagnose reports only Packy core availability. Active-pack health is added
// separately; removed classic state and projections are intentionally ignored.
func Diagnose(homeDir, configHome string) Report {
	return Report{
		SchemaVersion: 2,
		Kind:          "doctor",
		Context:       Context{HomeDir: homeDir, ConfigHome: configHome},
		Checks: []Check{{
			Name:     "packy-core",
			Severity: Pass,
			Detail:   "Packy core is available; no capability-pack activation is implied",
		}},
		Summary: Summary{Status: "healthy", Passes: 1},
	}
}
