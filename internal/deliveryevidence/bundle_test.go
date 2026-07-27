package deliveryevidence

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture() Bundle {
	criteria := []string{"AC-a", "AC-b"}
	rows := make([]AcceptanceRow, 0, len(criteria))
	for _, id := range criteria {
		rows = append(rows, AcceptanceRow{Identity: id, Criterion: "criterion " + id, OwningSeam: "module", PositiveEvidence: "positive", NegativeEvidence: "negative", FailureEvidence: "failure", MutationEvidence: "mutation", CompatibilityEvidence: "compatible", PreservationEvidence: "preserved", MigrationEvidence: "N/A: no migration", State: "proved"})
	}
	return Bundle{Schema: SchemaV1, Repository: RepositoryIdentity{"owner", "repo", "R_node"}, Issue: IssueIdentity{276, "I_node"}, Spec: SpecIdentity{277, "S_node"}, Authority: Authority{IssueSHA256: strings.Repeat("a", 64), SpecSHA256: strings.Repeat("b", 64), Labels: []string{"status:approved", "feature"}, DependencyDisposition: []DependencyDisposition{{"#275", "satisfied"}}, AcceptanceCriteria: criteria}, Scope: ScopeLedger{OwnedNow: []LedgerEntry{{"O1", "owned", "issue#276"}}, Deferred: []DeferredEntry{{"D1", "deferred", "issue#277", "owner-team"}}, Forbidden: []LedgerEntry{{"F1", "forbidden", "issue#276"}}, Prerequisites: []PrerequisiteEntry{{"E1", "prerequisite", "issue#275", "satisfied", "requalify on change"}}}, AcceptanceMatrix: rows, StartingBaseSHA: strings.Repeat("c", 40), Iterations: []Iteration{}}
}

func TestCanonicalRoundTripAndPermutation(t *testing.T) {
	a := fixture()
	b := fixture()
	b.Authority.Labels = []string{"feature", "status:approved"}
	b.AcceptanceMatrix[0], b.AcceptanceMatrix[1] = b.AcceptanceMatrix[1], b.AcceptanceMatrix[0]
	one, err := CanonicalJSON(a)
	if err != nil {
		t.Fatal(err)
	}
	two, err := CanonicalJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one, two) {
		t.Fatalf("canonical encodings differ\n%s\n%s", one, two)
	}
	got, err := Decode(one)
	if err != nil {
		t.Fatal(err)
	}
	want := a
	canonicalize(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip differs: %#v", got)
	}
}

func TestDecodeFailsClosed(t *testing.T) {
	base, _ := CanonicalJSON(fixture())
	tests := map[string][]byte{"unknown": bytes.Replace(base, []byte(`"schema":`), []byte(`"unknown":1,"schema":`), 1), "second": append(append([]byte{}, base...), []byte("{}\n")...), "noncanonical": bytes.Replace(base, []byte(`,"issue":`), []byte(", \"issue\":"), 1)}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(data); err == nil {
				t.Fatal("accepted invalid encoding")
			}
		})
	}
	b := fixture()
	b.AcceptanceMatrix = b.AcceptanceMatrix[:1]
	if _, err := CanonicalJSON(b); err == nil {
		t.Fatal("accepted missing row")
	}
	b = fixture()
	b.Scope.Deferred = []DeferredEntry{{"O1", "duplicate", "issue#1", "owner"}}
	if _, err := CanonicalJSON(b); err == nil {
		t.Fatal("accepted contradictory ledger")
	}
}

func TestCriteriaAndIterationIdentitiesAreStrict(t *testing.T) {
	b := fixture()
	b.Authority.AcceptanceCriteria = []string{"AC-a", "AC-a"}
	if err := Validate(b); err == nil {
		t.Fatal("duplicate criterion accepted")
	}
	b = fixture()
	b.AcceptanceMatrix = append(b.AcceptanceMatrix, AcceptanceRow{Identity: "foreign", Criterion: "x", OwningSeam: "x", PositiveEvidence: "x", NegativeEvidence: "x", FailureEvidence: "x", MutationEvidence: "x", CompatibilityEvidence: "x", PreservationEvidence: "x", MigrationEvidence: "N/A: x", State: "planned"})
	if err := Validate(b); err == nil {
		t.Fatal("foreign row accepted")
	}
	b = fixture()
	b.Iterations = []Iteration{{Identity: "iteration-1", BaseSHA: strings.Repeat("1", 40), HeadSHA: "", EvidenceSHA256: strings.Repeat("2", 64)}}
	if err := Validate(b); err == nil {
		t.Fatal("missing iteration head accepted")
	}
	b.Iterations[0].HeadSHA = strings.Repeat("3", 40)
	if err := Validate(b); err != nil {
		t.Fatal(err)
	}
	b.Spec.NodeID = b.Issue.NodeID
	if err := Validate(b); err == nil {
		t.Fatal("shared issue/spec identity accepted")
	}
}

func TestTypedLedgerAndMatrixFieldsAreRequired(t *testing.T) {
	b := fixture()
	b.Scope.Deferred[0].Owner = ""
	if err := Validate(b); err == nil {
		t.Fatal("ownerless deferred entry accepted")
	}
	b = fixture()
	b.Scope.Prerequisites[0].ExceptionBoundary = ""
	if err := Validate(b); err == nil {
		t.Fatal("boundaryless prerequisite accepted")
	}
	b = fixture()
	b.AcceptanceMatrix[0].State = "done"
	if err := Validate(b); err == nil {
		t.Fatal("invalid matrix state accepted")
	}
	b = fixture()
	b.AcceptanceMatrix[0].NegativeEvidence = ""
	if err := Validate(b); err == nil {
		t.Fatal("missing negative evidence accepted")
	}
}

func TestInitializeResumeStaleAndAtomicFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issue.json")
	b := fixture()
	r, err := InitializeOrResume(path, b)
	if err != nil || r.State != Initialized {
		t.Fatalf("initialize: %#v %v", r, err)
	}
	old, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	r, err = InitializeOrResume(path, b)
	if err != nil || r.State != Resumed {
		t.Fatalf("resume: %#v %v", r, err)
	}
	changed := fixture()
	changed.Authority.SpecSHA256 = strings.Repeat("d", 64)
	r, err = InitializeOrResume(path, changed)
	if err != nil || r.State != Stale {
		t.Fatalf("stale: %#v %v", r, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(old, after) {
		t.Fatal("stale authority overwrote evidence")
	}
	if err = StoreAtomic(path, changed); err != nil {
		t.Fatal(err)
	}
	newBytes, _ := os.ReadFile(path)
	if bytes.Equal(old, newBytes) {
		t.Fatal("atomic replacement did not install new evidence")
	}
	old = newBytes
	err = StoreAtomicWithRename(path, changed, func(string, string) error { return errors.New("fault") })
	if err == nil {
		t.Fatal("fault accepted")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(old, after) {
		t.Fatal("rename fault damaged old evidence")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func TestResolvePathAndStatusAreSanitized(t *testing.T) {
	common := filepath.Join(t.TempDir(), "common")
	p, err := ResolvePath(common, "", 276)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(common, "packy", "issue-delivery", "issue-276.json") {
		t.Fatal(p)
	}
	if _, err = ResolvePath(common, "relative", 276); err == nil {
		t.Fatal("relative override accepted")
	}
	s, err := RenderStatus(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"owned_now=1 deferred=1 forbidden=1 prerequisites=1", "planned=0 implemented=0 proved=2", "Iterations: 0"} {
		if !strings.Contains(s, want) {
			t.Fatalf("status missing %q: %s", want, s)
		}
	}
	for _, forbidden := range []string{"body", "credential", "HOME", "XDG_CONFIG_HOME"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("status leaked %s", forbidden)
		}
	}
}
