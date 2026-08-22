package offlinevalidation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == ModeArgument {
		os.Exit(Run(os.Args[2:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func TestAdapterValidateRunsTheProductionWorkerProcess(t *testing.T) {
	acquisition := writeAcquisitionFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	validation, err := New(executable).Validate(context.Background(), acquisition)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if validation.Manifest.ID != "example" || validation.Manifest.Version != "1.0.0" || validation.ClosureSHA256 == "" {
		t.Fatalf("validation = %#v", validation)
	}
}

func TestAdapterValidateRunsTheExactExecutableWithAnIsolatedEnvironment(t *testing.T) {
	t.Setenv("GH_TOKEN", "github-secret")
	t.Setenv("GITHUB_TOKEN", "other-secret")
	t.Setenv("GIT_ASKPASS", "/credential/helper")
	t.Setenv("SSH_AUTH_SOCK", "/credential/socket")
	t.Setenv("HTTPS_PROXY", "https://proxy.example")
	t.Setenv("http_proxy", "http://proxy.example")
	acquisition := writeAcquisitionFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	wantValidation := fakeValidation()
	runner := processRunnerFunc(func(ctx context.Context, invocation processInvocation) error {
		deadline, bounded := ctx.Deadline()
		if !bounded || time.Until(deadline) <= 0 || time.Until(deadline) > workerTimeout {
			t.Fatalf("worker context deadline = %v, bounded = %t", deadline, bounded)
		}
		if invocation.Executable != executable {
			t.Fatalf("executable = %q, want %q", invocation.Executable, executable)
		}
		if len(invocation.Args) != 3 || invocation.Args[0] != ModeArgument {
			t.Fatalf("args = %#v", invocation.Args)
		}
		if invocation.Dir != filepath.Dir(invocation.Args[1]) {
			t.Fatalf("dir = %q, request = %q", invocation.Dir, invocation.Args[1])
		}

		environment := environmentMap(t, invocation.Env)
		wantKeys := []string{"HOME", "LANG", "LC_ALL", "PATH", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"}
		gotKeys := make([]string, 0, len(environment))
		for key := range environment {
			gotKeys = append(gotKeys, key)
		}
		sort.Strings(gotKeys)
		if !reflect.DeepEqual(gotKeys, wantKeys) {
			t.Fatalf("environment keys = %#v, want %#v", gotKeys, wantKeys)
		}
		for _, key := range []string{"HOME", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
			if !pathWithin(invocation.Dir, environment[key]) {
				t.Fatalf("%s = %q is outside %q", key, environment[key], invocation.Dir)
			}
			info, statErr := os.Lstat(environment[key])
			if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("%s directory = %q, info = %#v, error = %v", key, environment[key], info, statErr)
			}
		}
		for key, value := range environment {
			upper := strings.ToUpper(key)
			if strings.HasPrefix(upper, "GH_") || strings.HasPrefix(upper, "GITHUB_") || strings.HasPrefix(upper, "GIT_") || strings.HasPrefix(upper, "SSH_") || strings.Contains(upper, "PROXY") || strings.Contains(upper, "CREDENTIAL") || strings.Contains(value, "secret") || strings.Contains(value, "credential") || strings.Contains(value, "proxy.example") {
				t.Fatalf("unsafe environment entry %s=%q", key, value)
			}
		}

		request, readErr := readWorkerRequest(invocation.Args[1], invocation.Args[2])
		if readErr != nil {
			t.Fatal(readErr)
		}
		response := workerResponse{
			Protocol: request.Protocol, Token: request.Token, Nonce: request.Nonce,
			RequestSHA256: request.RequestSHA256, Status: responseAccepted,
			Validation: wantValidation, ValidationSHA256: validationDigest(wantValidation),
		}
		response.ResponseSHA256 = responseDigest(response)
		return writeWorkerResponse(invocation.Args[2], response)
	})

	got, err := newWithRunner(executable, runner).Validate(context.Background(), acquisition)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !reflect.DeepEqual(got, wantValidation) {
		t.Fatalf("validation = %#v, want %#v", got, wantValidation)
	}
}

func TestAdapterValidatePropagatesCancellationAndRemovesProtocolState(t *testing.T) {
	acquisition := writeAcquisitionFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var protocolRoot string
	var cancel context.CancelFunc
	runner := processRunnerFunc(func(ctx context.Context, invocation processInvocation) error {
		protocolRoot = invocation.Dir
		cancel()
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = newWithRunner(executable, runner).Validate(ctx, acquisition)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Validate() error = %v", err)
	}
	if protocolRoot != "" {
		if _, statErr := os.Stat(protocolRoot); !os.IsNotExist(statErr) {
			t.Fatalf("protocol root still exists: %v", statErr)
		}
	}
}

func TestAdapterValidateRejectsStaleOrTamperedWorkerOutputAsOperationalFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*workerResponse)
		want string
	}{
		{
			name: "stale identity",
			edit: func(response *workerResponse) {
				response.Nonce = strings.Repeat("e", nonceBytes*2)
				response.ResponseSHA256 = responseDigest(*response)
			},
			want: "identity",
		},
		{
			name: "tampered validation",
			edit: func(response *workerResponse) {
				response.Validation.Manifest.ID = "tampered"
			},
			want: "response digest",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			acquisition := writeAcquisitionFixture(t)
			runner := processRunnerFunc(func(_ context.Context, invocation processInvocation) error {
				request, readErr := readWorkerRequest(invocation.Args[1], invocation.Args[2])
				if readErr != nil {
					return readErr
				}
				validation := fakeValidation()
				response := workerResponse{
					Protocol: request.Protocol, Token: request.Token, Nonce: request.Nonce,
					RequestSHA256: request.RequestSHA256, Status: responseAccepted,
					Validation: validation, ValidationSHA256: validationDigest(validation),
				}
				response.ResponseSHA256 = responseDigest(response)
				test.edit(&response)
				return writeWorkerResponse(invocation.Args[2], response)
			})

			_, err := newWithRunner(executable, runner).Validate(context.Background(), acquisition)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
			var rejection *managedpackpromotion.RejectionError
			if errors.As(err, &rejection) {
				t.Fatalf("tampered protocol output became policy rejection: %#v", rejection)
			}
		})
	}
}

func TestAdapterValidateConvertsWorkerPolicyRejectionToTypedGate(t *testing.T) {
	acquisition := writeAcquisitionFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runner := processRunnerFunc(func(_ context.Context, invocation processInvocation) error {
		request, readErr := readWorkerRequest(invocation.Args[1], invocation.Args[2])
		if readErr != nil {
			return readErr
		}
		response := workerResponse{
			Protocol: request.Protocol, Token: request.Token, Nonce: request.Nonce,
			RequestSHA256: request.RequestSHA256, Status: responseRejected,
			Gate: managedpackpromotion.GateExactCopies, Reason: "origin bytes drifted",
		}
		response.ResponseSHA256 = responseDigest(response)
		return writeWorkerResponse(invocation.Args[2], response)
	})

	_, err = newWithRunner(executable, runner).Validate(context.Background(), acquisition)
	var rejection *managedpackpromotion.RejectionError
	if !errors.As(err, &rejection) || rejection.Gate != managedpackpromotion.GateExactCopies || rejection.Reason != "origin bytes drifted" {
		t.Fatalf("Validate() error = %v, rejection = %#v", err, rejection)
	}
}

func fakeValidation() managedpack.Validation {
	digest := strings.Repeat("c", 64)
	return managedpack.Validation{
		Manifest:       managedpack.Manifest{SchemaVersion: 1, ID: "example", Version: "1.0.0"},
		ManifestSHA256: digest,
		ClosureSHA256:  strings.Repeat("d", 64),
		Files:          []managedpack.FileRecord{{Path: "pack.json", Mode: "100644", SHA256: digest}},
	}
}

func writeAcquisitionFixture(t *testing.T) managedpackpromotion.Acquisition {
	t.Helper()
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	originRoot := filepath.Join(root, "origins", "upstream")
	writeTestFile(t, filepath.Join(projectRoot, "skills", "guide", "SKILL.md"), "managed guidance\n", 0o644)
	writeTestFile(t, filepath.Join(projectRoot, "notices", "mit"), "MIT notice\n", 0o644)
	writeTestFile(t, filepath.Join(originRoot, "guide", "SKILL.md"), "managed guidance\n", 0o644)
	writeTestFile(t, filepath.Join(projectRoot, "pack.json"), validManifest, 0o644)
	return managedpackpromotion.Acquisition{ProjectRoot: projectRoot, OriginRoots: map[string]string{"upstream": originRoot}}
}

func environmentMap(t *testing.T, environment []string) map[string]string {
	t.Helper()
	result := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			t.Fatalf("invalid environment entry %q", entry)
		}
		if _, exists := result[key]; exists {
			t.Fatalf("duplicate environment key %q", key)
		}
		result[key] = value
	}
	return result
}
