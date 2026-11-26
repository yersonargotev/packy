// Package githubauth finds the GitHub credential that an explicit catalog
// network operation sends. Packy never stores the credential.
package githubauth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ghSource = "gh auth token"

// ErrMissing reports that no source holds a GitHub token.
var ErrMissing = errors.New("no GitHub token is available; set GH_TOKEN or GITHUB_TOKEN, or sign in with `gh auth login`")

// ghEnvironment names the variables that `gh auth token` needs to find its
// configuration and the system keyring.
var ghEnvironment = []string{"PATH", "HOME", "XDG_CONFIG_HOME", "GH_CONFIG_DIR", "DBUS_SESSION_BUS_ADDRESS", "XDG_RUNTIME_DIR"}

// Token is a GitHub credential. It formats as the name of its source, never as
// its value.
type Token struct{ value, source string }

func (t Token) String() string { return t.source }

func (t Token) GoString() string { return "githubauth.Token(" + t.source + ")" }

// Renewal says how to replace the token after GitHub rejects it.
func (t Token) Renewal() string {
	if t.source == ghSource {
		return "sign in again with `gh auth login`"
	}
	return "replace or unset " + t.source
}

// Authorize sends the token as the bearer credential of request.
func (t Token) Authorize(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+t.value)
}

// Lookup returns the token from GH_TOKEN, then GITHUB_TOKEN, then
// `gh auth token` for github.com. getenv supplies the environment, and gh runs
// with only the variables it needs from it.
func Lookup(ctx context.Context, getenv func(string) string) (Token, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			return Token{value: value, source: name}, nil
		}
	}
	gh, err := lookPath(getenv("PATH"), "gh")
	if err != nil {
		return Token{}, ErrMissing
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, gh, "auth", "token", "--hostname", "github.com")
	for _, name := range ghEnvironment {
		if value := getenv(name); value != "" {
			command.Env = append(command.Env, name+"="+value)
		}
	}
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		return Token{}, fmt.Errorf("%w; gh auth token failed: %v", ErrMissing, err)
	}
	value := strings.TrimSpace(stdout.String())
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return Token{}, fmt.Errorf("%w; gh auth token printed no single token", ErrMissing)
	}
	return Token{value: value, source: ghSource}, nil
}

// lookPath searches the PATH from getenv, not the process PATH, and skips
// relative entries so that a gh in the working directory never runs.
func lookPath(path, name string) (string, error) {
	for _, directory := range filepath.SplitList(path) {
		if !filepath.IsAbs(directory) {
			continue
		}
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}
