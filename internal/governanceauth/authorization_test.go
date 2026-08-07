package governanceauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	Event    Event    `json:"event"`
	Metadata Metadata `json:"metadata"`
	Want     string   `json:"want_error"`
}

func TestAuthorizationFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no authorization fixtures found")
	}

	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var test fixture
			if err := json.Unmarshal(contents, &test); err != nil {
				t.Fatal(err)
			}

			err = Validate(test.Event, test.Metadata)
			if test.Want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.Want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.Want)
			}
		})
	}
}

func TestPackSourceAutomationRequiresOneApprovedClosingIssue(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("testdata", "sync-automation-approved-issue.json"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*fixture)
		want   string
	}{
		{
			name: "missing snapshot",
			mutate: func(test *fixture) {
				test.Metadata.Issues = nil
			},
			want: "snapshots do not match",
		},
		{
			name: "closed issue",
			mutate: func(test *fixture) {
				test.Metadata.Issues[0].State = "CLOSED"
			},
			want: "is not open",
		},
		{
			name: "unapproved issue",
			mutate: func(test *fixture) {
				test.Metadata.Issues[0].Labels = nil
			},
			want: "does not have exactly one approved delivery status",
		},
		{
			name: "multiple issues",
			mutate: func(test *fixture) {
				test.Metadata.ClosingIssuesReferences = append(test.Metadata.ClosingIssuesReferences, test.Metadata.ClosingIssuesReferences[0])
			},
			want: "must close exactly one approved issue",
		},
		{
			name: "cross repository issue",
			mutate: func(test *fixture) {
				test.Metadata.ClosingIssuesReferences[0].Repository.Name = "other"
			},
			want: "does not match",
		},
		{
			name: "stale issue snapshot",
			mutate: func(test *fixture) {
				test.Metadata.Issues[0].Number++
			},
			want: "trusted snapshot for closing issue",
		},
		{
			name: "release automation",
			mutate: func(test *fixture) {
				test.Event.PullRequest.HeadRef = "release/v1.0.0"
				test.Metadata.Exception.Workflow = "Release"
				test.Metadata.Exception.Path = ".github/workflows/release.yml"
			},
			want: "cannot be mixed",
		},
		{
			name: "dependabot automation",
			mutate: func(test *fixture) {
				test.Event.PullRequest.Author = "app/dependabot"
				test.Event.PullRequest.HeadRef = "dependabot/go_modules/example-1.2.3"
				test.Metadata.Exception.Kind = "dependabot-pr"
			},
			want: "cannot be mixed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var candidate fixture
			if err := json.Unmarshal(contents, &candidate); err != nil {
				t.Fatal(err)
			}
			test.mutate(&candidate)
			err := Validate(candidate.Event, candidate.Metadata)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
