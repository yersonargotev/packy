package cli

import (
	"context"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/engrambin"
)

type formulaOutputRunner struct {
	*fakeRunner
	stdout string
	stderr string
	exit   int
	err    error
}

func (r *formulaOutputRunner) RunOutput(_ context.Context, name string, args ...string) (string, string, int, error) {
	r.calls = append(r.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	return r.stdout, r.stderr, r.exit, r.err
}

func TestHomebrewFormulaInspectorResolvesImmutableAcquisitionFactsReadOnly(t *testing.T) {
	runner := &formulaOutputRunner{
		fakeRunner: &fakeRunner{},
		stdout:     `{"formulae":[{"full_name":"gentleman-programming/tap/engram","versions":{"stable":"0.4.2"}}]}`,
	}
	metadata, err := inspectHomebrewFormula(context.Background(), runner, engrambin.Formula)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Source != engrambin.Formula || metadata.Version != "0.4.2" {
		t.Fatalf("metadata = %+v", metadata)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != "brew" || !reflect.DeepEqual(runner.calls[0].args, []string{"info", "--json=v2", engrambin.Formula}) {
		t.Fatalf("read-only inspection calls = %#v", runner.calls)
	}
}
