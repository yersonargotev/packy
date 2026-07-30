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

type faultFile struct {
	atomicFile
	stage string
}

func (f faultFile) Chmod(m os.FileMode) error {
	if f.stage == "chmod" {
		return errors.New("fault")
	}
	return f.atomicFile.Chmod(m)
}
func (f faultFile) Write(b []byte) (int, error) {
	if f.stage == "write" {
		return 0, errors.New("fault")
	}
	return f.atomicFile.Write(b)
}
func (f faultFile) Sync() error {
	if f.stage == "file-sync" {
		return errors.New("fault")
	}
	return f.atomicFile.Sync()
}
func (f faultFile) Close() error {
	if f.stage == "file-close" {
		_ = f.atomicFile.Close()
		return errors.New("fault")
	}
	return f.atomicFile.Close()
}

type faultDirectory struct {
	atomicDirectory
	stage string
}

func (f faultDirectory) Sync() error {
	if f.stage == "directory-sync" {
		return errors.New("fault")
	}
	return f.atomicDirectory.Sync()
}
func (f faultDirectory) Close() error {
	if f.stage == "directory-close" {
		_ = f.atomicDirectory.Close()
		return errors.New("fault")
	}
	return f.atomicDirectory.Close()
}

func fixture() Bundle {
	criteria := []string{"AC-a", "AC-b"}
	rows := make([]AcceptanceRow, 0, len(criteria))
	for _, id := range criteria {
		rows = append(rows, AcceptanceRow{Identity: id, Criterion: "criterion " + id, OwningSeam: "module", PositiveEvidence: "positive", NegativeEvidence: "negative", FailureEvidence: "failure", MutationEvidence: "mutation", CompatibilityEvidence: "compatible", PreservationEvidence: "preserved", MigrationEvidence: "N/A: no migration", State: "proved"})
	}
	return Bundle{Schema: SchemaV1, Repository: RepositoryIdentity{"owner", "repo", "R_node"}, Issue: IssueIdentity{276, "I_node"}, Spec: SpecIdentity{277, "S_node"}, Authority: Authority{IssueSHA256: strings.Repeat("a", 64), SpecSHA256: strings.Repeat("b", 64), Labels: []string{"status:approved", "feature"}, DependencyDisposition: []DependencyDisposition{{"#275", "satisfied"}}, AcceptanceCriteria: criteria}, Scope: ScopeLedger{OwnedNow: []LedgerEntry{{"O1", "owned", "issue#276"}}, Deferred: []DeferredEntry{{"D1", "deferred", "issue#277", "owner-team"}}, Forbidden: []LedgerEntry{{"F1", "forbidden", "issue#276"}}, Prerequisites: []PrerequisiteEntry{{"E1", "prerequisite", "issue#275", "satisfied", "requalify on change"}}}, AcceptanceMatrix: rows, StartingBaseSHA: strings.Repeat("c", 40), Iterations: []Iteration{}}
}

func v2Fixture(kind DeliveryAuthorityKind, profile DeliveryRiskProfile) Bundle {
	b := fixture()
	b.Schema = SchemaV2
	b.Authority.Kind = kind
	b.RiskProfile = profile
	if kind == AuthoritySelfContainedIssue {
		b.Spec = SpecIdentity{}
		b.Authority.SpecSHA256 = ""
	}
	return b
}

func TestV2AuthorityRiskAndCanonicalPersistence(t *testing.T) {
	for _, kind := range []DeliveryAuthorityKind{AuthoritySelfContainedIssue, AuthorityIssueWithSpecification} {
		for _, profile := range []DeliveryRiskProfile{RiskLow, RiskStandard, RiskHigh} {
			t.Run(string(kind)+"/"+string(profile), func(t *testing.T) {
				want := v2Fixture(kind, profile)
				data, err := CanonicalJSON(want)
				if err != nil {
					t.Fatal(err)
				}
				if kind == AuthoritySelfContainedIssue && (bytes.Contains(data, []byte(`"spec":`)) || bytes.Contains(data, []byte(`"spec_sha256":`))) {
					t.Fatalf("self-contained authority manufactured specification evidence: %s", data)
				}
				path := filepath.Join(t.TempDir(), "evidence.json")
				if err := StoreAtomic(path, want); err != nil {
					t.Fatal(err)
				}
				got, stored, err := Load(path)
				if err != nil {
					t.Fatal(err)
				}
				canonicalize(&want)
				if !reflect.DeepEqual(got, want) || !bytes.Equal(stored, data) {
					t.Fatalf("v2 persistence roundtrip differs: %#v", got)
				}
				status, err := RenderStatus(got)
				if err != nil || !strings.Contains(status, string(kind)) || !strings.Contains(status, string(profile)) {
					t.Fatalf("v2 status: %q %v", status, err)
				}
			})
		}
	}
}

func TestV2RequiresExplicitValidAuthorityAndRisk(t *testing.T) {
	tests := map[string]func(*Bundle){
		"missing authority": func(b *Bundle) { b.Authority.Kind = "" },
		"invalid authority": func(b *Bundle) { b.Authority.Kind = "other" },
		"missing profile":   func(b *Bundle) { b.RiskProfile = "" },
		"invalid profile":   func(b *Bundle) { b.RiskProfile = "routine" },
		"self spec": func(b *Bundle) {
			b.Spec = SpecIdentity{277, "S_node"}
		},
		"self spec digest": func(b *Bundle) { b.Authority.SpecSHA256 = strings.Repeat("b", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := v2Fixture(AuthoritySelfContainedIssue, RiskStandard)
			mutate(&b)
			if err := Validate(b); err == nil {
				t.Fatal("accepted invalid v2 bundle")
			}
		})
	}

	b := v2Fixture(AuthorityIssueWithSpecification, RiskStandard)
	b.Authority.SpecSHA256 = ""
	if err := Validate(b); err == nil {
		t.Fatal("issue-with-specification accepted without spec digest")
	}
}

func TestV1AndV2QualificationAreStaleNotConverted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issue.json")
	v1 := fixture()
	if err := StoreAtomic(path, v1); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	result, err := InitializeOrResume(path, v2Fixture(AuthorityIssueWithSpecification, RiskStandard))
	if err != nil || result.State != Stale {
		t.Fatalf("v1 versus v2 = %#v, %v", result, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("stale v1 evidence was converted or overwritten")
	}
}

func TestCompileQualificationOwnsAuthorityAndRiskPolicy(t *testing.T) {
	tests := []struct {
		name    string
		input   QualificationInput
		want    QualificationPlan
		wantErr bool
	}{
		{
			name:  "v1 legacy authority",
			input: QualificationInput{Schema: SchemaV1, IssueNumber: 355, SpecNumber: 354},
			want:  QualificationPlan{HasSpecification: true},
		},
		{
			name:  "v2 self contained defaults standard",
			input: QualificationInput{Schema: SchemaV2, IssueNumber: 355, AuthorityKind: AuthoritySelfContainedIssue},
			want:  QualificationPlan{AuthorityKind: AuthoritySelfContainedIssue, RiskProfile: RiskStandard},
		},
		{
			name:  "v2 issue with specification",
			input: QualificationInput{Schema: SchemaV2, IssueNumber: 355, SpecNumber: 354, AuthorityKind: AuthorityIssueWithSpecification, RiskProfile: RiskHigh},
			want:  QualificationPlan{AuthorityKind: AuthorityIssueWithSpecification, RiskProfile: RiskHigh, HasSpecification: true},
		},
		{
			name:    "v2 invalid risk",
			input:   QualificationInput{Schema: SchemaV2, IssueNumber: 355, AuthorityKind: AuthoritySelfContainedIssue, RiskProfile: "routine"},
			wantErr: true,
		},
		{
			name:    "self contained with specification",
			input:   QualificationInput{Schema: SchemaV2, IssueNumber: 355, SpecNumber: 354, AuthorityKind: AuthoritySelfContainedIssue},
			wantErr: true,
		},
		{
			name:    "v1 with v2 fields",
			input:   QualificationInput{Schema: SchemaV1, IssueNumber: 355, SpecNumber: 354, RiskProfile: RiskStandard},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CompileQualification(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("CompileQualification() error = %v, wantErr %t", err, test.wantErr)
			}
			if !test.wantErr && got != test.want {
				t.Fatalf("CompileQualification() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestV2IsNotAdmittedToLegacyWorkflow(t *testing.T) {
	if err := ValidateLegacyWorkflowBundle(fixture()); err != nil {
		t.Fatalf("v1 rejected from legacy workflow: %v", err)
	}
	if err := ValidateLegacyWorkflowBundle(v2Fixture(AuthoritySelfContainedIssue, RiskStandard)); err == nil {
		t.Fatal("v2 admitted to legacy workflow before Advance")
	}
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
	b.Iterations = []Iteration{{Sequence: 1, Identity: "iteration-1", BaseSHA: b.StartingBaseSHA, HeadSHA: "", EvidenceSHA256: strings.Repeat("2", 64)}}
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

func TestIterationChainAndSafeText(t *testing.T) {
	b := fixture()
	head := strings.Repeat("d", 40)
	b.Iterations = []Iteration{{Sequence: 2, Identity: "second", BaseSHA: head, HeadSHA: strings.Repeat("e", 40), EvidenceSHA256: strings.Repeat("2", 64)}, {Sequence: 1, Identity: "first", BaseSHA: b.StartingBaseSHA, HeadSHA: head, EvidenceSHA256: strings.Repeat("1", 64)}}
	data, err := CanonicalJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil || got.Iterations[0].Sequence != 1 {
		t.Fatalf("iteration order: %v %#v", err, got.Iterations)
	}
	b = fixture()
	b.Iterations = []Iteration{{Sequence: 1, Identity: "first", BaseSHA: strings.Repeat("f", 40), HeadSHA: head, EvidenceSHA256: strings.Repeat("1", 64)}}
	if err := Validate(b); err == nil {
		t.Fatal("disconnected chain accepted")
	}
	for _, unsafe := range []string{"line\nbreak", "-----BEGIN PRIVATE KEY-----", "ghp_abcdefghijklmnopqrstuvwxyz", "Authorization: Bearer x", "GITHUB_TOKEN=secret", "UPSTREAM_PAYLOAD=bytes", "password=hunter2"} {
		b = fixture()
		b.Scope.OwnedNow[0].Requirement = unsafe
		if err := Validate(b); err == nil {
			t.Fatalf("unsafe text accepted: %q", unsafe)
		}
	}
	b = fixture()
	b.Scope.OwnedNow[0].Requirement = "Must reject password assignment patterns without retaining them"
	if err := Validate(b); err != nil {
		t.Fatalf("neutral prose rejected: %v", err)
	}
	b = fixture()
	b.Authority.DependencyDisposition[0].Disposition = "unknown"
	if err := Validate(b); err == nil {
		t.Fatal("unknown dependency state accepted")
	}
}

func TestPersistedLabelsUseSafeText(t *testing.T) {
	for _, unsafe := range []string{"line\nbreak", "GITHUB_TOKEN=secret", "-----BEGIN PRIVATE KEY-----"} {
		b := fixture()
		b.Authority.Labels = append(b.Authority.Labels, unsafe)
		if _, err := CanonicalJSON(b); err == nil {
			t.Fatalf("unsafe label accepted: %q", unsafe)
		}
	}
	b := fixture()
	b.Authority.Labels = append(b.Authority.Labels, "area:delivery-evidence", "status:needs-review")
	if _, err := CanonicalJSON(b); err != nil {
		t.Fatalf("ordinary GitHub labels rejected: %v", err)
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
	ops := defaultAtomicOps()
	ops.Rename = func(string, string) error { return errors.New("fault") }
	err = storeAtomicWithOps(path, changed, ops)
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

func TestAtomicFaultBoundariesLeaveCompleteEvidence(t *testing.T) {
	for _, stage := range []string{"mkdir", "create", "chmod", "write", "file-sync", "file-close", "rename", "open-directory", "directory-sync", "directory-close"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "evidence.json")
			old := fixture()
			if err := StoreAtomic(path, old); err != nil {
				t.Fatal(err)
			}
			oldBytes, _ := CanonicalJSON(old)
			next := fixture()
			next.Authority.IssueSHA256 = strings.Repeat("f", 64)
			newBytes, _ := CanonicalJSON(next)
			ops := defaultAtomicOps()
			if stage == "mkdir" {
				ops.MkdirAll = func(string, os.FileMode) error { return errors.New("fault") }
			}
			create := ops.CreateTemp
			ops.CreateTemp = func(d, p string) (atomicFile, error) {
				if stage == "create" {
					return nil, errors.New("fault")
				}
				f, e := create(d, p)
				if e != nil {
					return nil, e
				}
				return faultFile{f, stage}, nil
			}
			if stage == "rename" {
				ops.Rename = func(string, string) error { return errors.New("fault") }
			}
			open := ops.OpenDirectory
			ops.OpenDirectory = func(p string) (atomicDirectory, error) {
				if stage == "open-directory" {
					return nil, errors.New("fault")
				}
				d, e := open(p)
				if e != nil {
					return nil, e
				}
				return faultDirectory{d, stage}, nil
			}
			if err := storeAtomicWithOps(path, next, ops); err == nil {
				t.Fatal("fault was ignored")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, oldBytes) && !bytes.Equal(got, newBytes) {
				t.Fatal("partial evidence visible")
			}
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".issue-delivery-") {
					t.Fatalf("temporary remains: %s", e.Name())
				}
			}
		})
	}
}

func TestAtomicRemoveFaultRetriesAndFailsClosed(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		name := "transient"
		if permanent {
			name = "permanent"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "evidence.json")
			old := fixture()
			if err := StoreAtomic(path, old); err != nil {
				t.Fatal(err)
			}
			oldBytes, _ := CanonicalJSON(old)
			next := fixture()
			next.Authority.IssueSHA256 = strings.Repeat("f", 64)
			ops := defaultAtomicOps()
			create := ops.CreateTemp
			ops.CreateTemp = func(d, p string) (atomicFile, error) {
				f, err := create(d, p)
				if err != nil {
					return nil, err
				}
				return faultFile{atomicFile: f, stage: "write"}, nil
			}
			remove := ops.Remove
			calls := 0
			ops.Remove = func(path string) error {
				calls++
				if permanent || calls == 1 {
					return errors.New("remove fault")
				}
				return remove(path)
			}
			err := storeAtomicWithOps(path, next, ops)
			if err == nil || !strings.Contains(err.Error(), "remove temporary") {
				t.Fatalf("cleanup failure not returned: %v", err)
			}
			if calls != 2 {
				t.Fatalf("remove calls = %d, want retry", calls)
			}
			loaded, got, err := Load(path)
			if err != nil || loaded.Authority.IssueSHA256 != old.Authority.IssueSHA256 || !bytes.Equal(got, oldBytes) {
				t.Fatalf("authoritative path changed: %v", err)
			}
			entries, _ := os.ReadDir(dir)
			temps := 0
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".issue-delivery-") {
					temps++
				}
			}
			if !permanent && temps != 0 {
				t.Fatalf("transient cleanup left %d temps", temps)
			}
			if permanent && temps != 1 {
				t.Fatalf("permanent cleanup residuals = %d, want reported temp", temps)
			}
		})
	}
}
