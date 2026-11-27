package githubauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLookupPrefersGHTokenThenGitHubTokenThenGH(t *testing.T) {
	bin, record := stubGH(t, "echo gh-value")
	for _, test := range []struct {
		name, source, value string
		env                 map[string]string
	}{
		{"GH_TOKEN wins", "GH_TOKEN", "gh-token-value", map[string]string{"GH_TOKEN": "gh-token-value", "GITHUB_TOKEN": "github-token-value"}},
		{"GITHUB_TOKEN before gh", "GITHUB_TOKEN", "github-token-value", map[string]string{"GH_TOKEN": "  ", "GITHUB_TOKEN": "github-token-value"}},
		{"gh last", "gh auth token", "gh-value", map[string]string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.env["PATH"] = bin
			token, err := Lookup(context.Background(), mapEnv(test.env))
			if err != nil {
				t.Fatal(err)
			}
			if token.String() != test.source || authorization(token) != "Bearer "+test.value {
				t.Fatalf("Lookup() = %s with %q, want %s with %q", token, authorization(token), test.source, test.value)
			}
		})
	}
	if args, err := os.ReadFile(filepath.Join(record, "args")); err != nil || strings.TrimSpace(string(args)) != "auth token --hostname github.com" {
		t.Fatalf("gh arguments = %q %v", args, err)
	}
}

func TestLookupRunsGHWithAnExplicitEnvironment(t *testing.T) {
	bin, record := stubGH(t, "echo gh-value")
	home := t.TempDir()
	if _, err := Lookup(context.Background(), mapEnv(map[string]string{"PATH": bin, "HOME": home, "GH_CONFIG_DIR": "/gh/config", "PACKY_AMBIENT_SECRET": "must-not-cross"})); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(record, "env"))
	if err != nil {
		t.Fatal(err)
	}
	var got, names []string
	for _, entry := range strings.Split(strings.TrimSpace(string(env)), "\n") {
		if name, _, _ := strings.Cut(entry, "="); !slices.Contains([]string{"PWD", "OLDPWD", "SHLVL", "_"}, name) {
			got, names = append(got, entry), append(names, name)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"GH_CONFIG_DIR=/gh/config", "HOME=" + home, "PATH=" + bin}) {
		t.Fatalf("gh environment has variables %q, want only GH_CONFIG_DIR, HOME, and PATH from getenv besides the variables sh sets", names)
	}
}

func TestLookupReportsAMissingToken(t *testing.T) {
	failing, _ := stubGH(t, "echo 'no oauth token found' >&2; exit 1")
	silent, _ := stubGH(t, "exit 0")
	for name, path := range map[string]string{"gh absent": t.TempDir(), "gh signed out": failing, "gh prints nothing": silent} {
		t.Run(name, func(t *testing.T) {
			_, err := Lookup(context.Background(), mapEnv(map[string]string{"PATH": path}))
			if !errors.Is(err, ErrMissing) || !strings.Contains(err.Error(), "gh auth login") {
				t.Fatalf("Lookup() error = %v", err)
			}
		})
	}
}

func TestTokenFormatsAsItsSource(t *testing.T) {
	token, err := Lookup(context.Background(), mapEnv(map[string]string{"GH_TOKEN": "secret-value"}))
	if err != nil {
		t.Fatal(err)
	}
	type holder struct {
		Exported Token
		Pointer  *Token
		hidden   Token
	}
	held := holder{token, &token, token}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%10s"} {
		want := "GH_TOKEN"
		if verb == "%#v" {
			want = "githubauth.Token(GH_TOKEN)"
		}
		if got := fmt.Sprintf(verb+" "+verb, token, &token); got != want+" "+want {
			t.Errorf("Sprintf(%q) of a token = %q, want %q", verb, got, want)
		}
		formatted := fmt.Sprintf(verb+" "+verb, held, []Token{token})
		for _, secret := range []string{"secret-value", fmt.Sprintf("%x", "secret-value"), fmt.Sprintf("%X", "secret-value")} {
			if strings.Contains(formatted, secret) {
				t.Errorf("Sprintf(%q) of a held token carries its value: %s", verb, formatted)
			}
		}
	}
}

func TestZeroTokenSendsNoCredential(t *testing.T) {
	if got := authorization(Token{}); got != "" {
		t.Fatalf("zero Token sent Authorization %q", got)
	}
}

func TestTokenRenewalNamesItsSource(t *testing.T) {
	bin, _ := stubGH(t, "echo gh-value")
	for env, want := range map[string]string{"GITHUB_TOKEN": "replace or unset GITHUB_TOKEN", "": "sign in again with `gh auth login`"} {
		token, err := Lookup(context.Background(), mapEnv(map[string]string{env: "value", "PATH": bin}))
		if err != nil || token.Renewal() != want {
			t.Fatalf("Renewal() of %s = %q, %v; want %q", token, token.Renewal(), err, want)
		}
	}
}

func stubGH(t *testing.T, body string) (bin, record string) {
	t.Helper()
	bin, record = t.TempDir(), t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\n/usr/bin/env > %q\necho \"$@\" > %q\n%s\n", filepath.Join(record, "env"), filepath.Join(record, "args"), body)
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, record
}

func authorization(token Token) string {
	request, _ := http.NewRequest(http.MethodGet, "https://api.github.com", nil)
	token.Authorize(request)
	return request.Header.Get("Authorization")
}

func mapEnv(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}
