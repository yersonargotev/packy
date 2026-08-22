package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/offlinevalidation"
)

func TestRunPromotesOneExactCoordinateAndRendersEachOutcome(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "promotepack")
	tests := []struct {
		name       string
		result     managedpackpromotion.Result
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name: "proposal",
			result: managedpackpromotion.Result{Status: managedpackpromotion.StatusProposal, Proposal: &managedpackpromotion.Proposal{
				Number: 17, URL: "https://github.com/example/packy/pull/17", Branch: "automation/example-1.2.3", HeadSHA: strings.Repeat("a", 40),
			}},
			wantStdout: "proposal https://github.com/example/packy/pull/17 branch=automation/example-1.2.3 head=" + strings.Repeat("a", 40) + "\n",
		},
		{
			name: "no change", result: managedpackpromotion.Result{Status: managedpackpromotion.StatusNoChange, Reason: "the exact admission already exists"},
			wantStdout: "no-change the exact admission already exists\n",
		},
		{
			name: "rejected", result: managedpackpromotion.Result{Status: managedpackpromotion.StatusRejected, Rejection: &managedpackpromotion.Rejection{
				Gate: managedpackpromotion.GateRelease, Reason: "release is mutable",
			}},
			wantExit: 1, wantStderr: "rejected gate=release reason=release is mutable\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakePromoter{result: test.result}
			deps := dependencies{
				executable:     func() (string, error) { return executable, nil },
				repositoryRoot: func(context.Context) (string, error) { return root, nil },
				newPromoter: func(got string) promoter {
					if got != executable {
						t.Fatalf("executable = %q", got)
					}
					return fake
				},
			}
			var stdout, stderr bytes.Buffer
			exit := run(context.Background(), []string{"example@1.2.3"}, &stdout, &stderr, deps)
			if exit != test.wantExit || stdout.String() != test.wantStdout || stderr.String() != test.wantStderr {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
			}
			if fake.request.RepositoryRoot != root || fake.request.Coordinate.String() != "example@1.2.3" {
				t.Fatalf("request = %#v", fake.request)
			}
		})
	}
}

func TestRunRejectsInvalidInvocationBeforeConstructingPromotion(t *testing.T) {
	for _, args := range [][]string{nil, {"example@1.0.0", "extra"}, {"not-a-coordinate"}} {
		var stdout, stderr bytes.Buffer
		exit := run(context.Background(), args, &stdout, &stderr, dependencies{})
		if exit != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), usage) {
			t.Fatalf("args = %q, exit = %d, stdout = %q, stderr = %q", args, exit, stdout.String(), stderr.String())
		}
	}
}

func TestRunRoutesTheHiddenWorkerModeWithoutPromotionDependencies(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := run(context.Background(), []string{offlinevalidation.ModeArgument}, &stdout, &stderr, dependencies{})
	if exit != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "offline validation worker requires") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
}

func TestRunReportsOperationalFailure(t *testing.T) {
	root := t.TempDir()
	fake := &fakePromoter{err: errors.New("network unavailable")}
	deps := dependencies{
		executable:     func() (string, error) { return filepath.Join(root, "promotepack"), nil },
		repositoryRoot: func(context.Context) (string, error) { return root, nil },
		newPromoter:    func(string) promoter { return fake },
	}
	var stdout, stderr bytes.Buffer
	exit := run(context.Background(), []string{"example@1.2.3"}, &stdout, &stderr, deps)
	if exit != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "network unavailable") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
}

type fakePromoter struct {
	result  managedpackpromotion.Result
	err     error
	request managedpackpromotion.Request
}

func (fake *fakePromoter) Promote(_ context.Context, request managedpackpromotion.Request) (managedpackpromotion.Result, error) {
	fake.request = request
	return fake.result, fake.err
}
