package offlinevalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestRunValidatesAcquiredLocalTreesAndSealsTheResponse(t *testing.T) {
	request, requestPath, responsePath := writeWorkerFixture(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{requestPath, responsePath}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %s", exitCode, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	response := readResponse(t, responsePath)
	if response.Protocol != protocolVersion || response.Token != request.Token || response.Nonce != request.Nonce || response.RequestSHA256 != request.RequestSHA256 {
		t.Fatalf("response identity = %#v", response)
	}
	if response.Status != responseAccepted || response.Gate != "" || response.Reason != "" {
		t.Fatalf("response outcome = %#v", response)
	}
	if response.Validation.Manifest.ID != "example" || response.Validation.Manifest.Version != "1.0.0" {
		t.Fatalf("validation manifest = %#v", response.Validation.Manifest)
	}
	if response.Validation.ManifestSHA256 == "" || response.Validation.ClosureSHA256 == "" || response.ValidationSHA256 == "" {
		t.Fatalf("response digests = %#v", response)
	}
}

func writeWorkerFixture(t *testing.T) (workerRequest, string, string) {
	t.Helper()
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	originRoot := filepath.Join(root, "origins", "upstream")
	writeTestFile(t, filepath.Join(projectRoot, "skills", "guide", "SKILL.md"), "managed guidance\n", 0o644)
	writeTestFile(t, filepath.Join(projectRoot, "notices", "mit"), "MIT notice\n", 0o644)
	writeTestFile(t, filepath.Join(originRoot, "guide", "SKILL.md"), "managed guidance\n", 0o644)
	writeTestFile(t, filepath.Join(projectRoot, "pack.json"), validManifest, 0o644)

	request := workerRequest{
		Protocol:    protocolVersion,
		Token:       strings.Repeat("a", tokenBytes*2),
		Nonce:       strings.Repeat("b", nonceBytes*2),
		ProjectRoot: projectRoot,
		OriginRoots: map[string]string{"upstream": originRoot},
	}
	request.RequestSHA256 = requestDigest(request)
	requestPath := filepath.Join(root, "request.json")
	responsePath := filepath.Join(root, "response.json")
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return request, requestPath, responsePath
}

func readResponse(t *testing.T, path string) workerResponse {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var response workerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestMapResolverNeverFallsBackBeyondAcquiredOrigins(t *testing.T) {
	_, err := (mapResolver{}).Resolve(context.Background(), managedpack.Origin{ID: "upstream"})
	if err == nil || !strings.Contains(err.Error(), "was not acquired") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestRunRejectsAnOriginReachedThroughASymlinkedAcquisitionDirectory(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	writeTestFile(t, filepath.Join(projectRoot, "pack.json"), validManifest, 0o644)
	externalOrigins := t.TempDir()
	writeTestFile(t, filepath.Join(externalOrigins, "upstream", "guide", "SKILL.md"), "managed guidance\n", 0o644)
	if err := os.Symlink(externalOrigins, filepath.Join(root, "origins")); err != nil {
		t.Fatal(err)
	}
	request := workerRequest{
		Protocol: protocolVersion, Token: strings.Repeat("a", tokenBytes*2), Nonce: strings.Repeat("b", nonceBytes*2),
		ProjectRoot: projectRoot, OriginRoots: map[string]string{"upstream": filepath.Join(root, "origins", "upstream")},
	}
	request.RequestSHA256 = requestDigest(request)
	requestPath := filepath.Join(root, "request.json")
	responsePath := filepath.Join(root, "response.json")
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if exitCode := Run([]string{requestPath, responsePath}, io.Discard, &stderr); exitCode != 1 {
		t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "origins") || !strings.Contains(stderr.String(), "symlink") {
		t.Fatalf("Run() stderr = %q", stderr.String())
	}
	if _, err := os.Stat(responsePath); !os.IsNotExist(err) {
		t.Fatalf("response file exists after rejected request: %v", err)
	}
}

func TestRunSealsValidationFailuresWithTheirTypedGate(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, workerRequest)
		gate managedpackpromotion.Gate
		want string
	}{
		{
			name: "contract",
			edit: func(t *testing.T, request workerRequest) {
				path := filepath.Join(request.ProjectRoot, "pack.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, path, strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 2`, 1), 0o644)
			},
			gate: managedpackpromotion.GateValidation,
			want: "schema_version",
		},
		{
			name: "origin",
			edit: func(t *testing.T, request workerRequest) {
				request.OriginRoots = map[string]string{}
				rewriteWorkerRequest(t, request)
			},
			gate: managedpackpromotion.GateOrigins,
			want: "was not acquired",
		},
		{
			name: "exact copy",
			edit: func(t *testing.T, request workerRequest) {
				writeTestFile(t, filepath.Join(request.ProjectRoot, "skills", "guide", "SKILL.md"), "drifted guidance\n", 0o644)
			},
			gate: managedpackpromotion.GateExactCopies,
			want: "exact-copy",
		},
		{
			name: "notice",
			edit: func(t *testing.T, request workerRequest) {
				path := filepath.Join(request.ProjectRoot, "pack.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, path, strings.Replace(string(data), `      "notices": ["notice:mit"],`+"\n", "", 1), 0o644)
			},
			gate: managedpackpromotion.GateNotices,
			want: "notice",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, requestPath, responsePath := writeWorkerFixture(t)
			test.edit(t, request)
			var stderr bytes.Buffer
			if exitCode := Run([]string{requestPath, responsePath}, io.Discard, &stderr); exitCode != 0 {
				t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
			}
			response := readResponse(t, responsePath)
			if response.Status != responseRejected || response.Gate != test.gate || !strings.Contains(response.Reason, test.want) {
				t.Fatalf("response = %#v, want gate %q and reason %q", response, test.gate, test.want)
			}
			if response.ResponseSHA256 != responseDigest(response) {
				t.Fatalf("response digest does not seal rejection: %#v", response)
			}
		})
	}
}

func rewriteWorkerRequest(t *testing.T, request workerRequest) {
	t.Helper()
	request.RequestSHA256 = requestDigest(request)
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(filepath.Dir(request.ProjectRoot), "request.json")
	if err := os.WriteFile(requestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

const validManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["claude", "codex", "opencode"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [
    {
      "id": "upstream",
      "repository": "example/upstream",
      "commit": "0123456789abcdef0123456789abcdef01234567",
      "revision": "v1.0.0"
    }
  ],
  "resources": [
    {
      "kind": "notice",
      "id": "mit",
      "source": "notices/mit",
      "description": "Preserves the MIT notice",
      "license": "MIT",
      "attribution": "Copyright Example",
      "requires": [],
      "conflicts": [],
      "bindings": [],
      "surface_exclusions": []
    },
    {
      "kind": "skill",
      "id": "guide",
      "source": "skills/guide",
      "description": "Provides managed guidance",
      "requires": [],
      "conflicts": [],
      "notices": ["notice:mit"],
      "origin": {
        "id": "upstream",
        "path": "guide",
        "relationship": "exact-copy"
      },
      "bindings": [
        {
          "surface": "claude",
          "projection": "skill",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": []
        },
        {
          "surface": "codex",
          "projection": "skill",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": []
        },
        {
          "surface": "opencode",
          "projection": "skill",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": []
        }
      ],
      "surface_exclusions": []
    }
  ]
}`
