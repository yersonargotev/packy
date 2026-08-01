package setuphealth

import (
	"reflect"
	"testing"
)

func TestDiagnoseIgnoresRemovedClassicStateAndProjections(t *testing.T) {
	want := Report{
		SchemaVersion: 2,
		Kind:          "doctor",
		Context:       Context{HomeDir: "/sandbox/home", ConfigHome: "/sandbox/xdg"},
		Checks: []Check{{
			Name:     "packy-core",
			Severity: Pass,
			Detail:   "Packy core is available; no capability-pack activation is implied",
		}},
		Summary: Summary{Status: "healthy", Passes: 1},
	}
	if got := Diagnose("/sandbox/home", "/sandbox/xdg"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Diagnose() = %#v, want %#v", got, want)
	}
}
