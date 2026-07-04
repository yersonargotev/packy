package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type fakeRunner struct {
	calls []fakeCall
}

type fakeCall struct {
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	return nil
}

func executeCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func sandboxOptions(t *testing.T) (Options, *fakeRunner, string) {
	t.Helper()
	home := t.TempDir()
	runner := &fakeRunner{}
	return Options{
		Env: MapEnv{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"),
		},
		Runner: runner,
	}, runner, home
}

func TestHelpRendersForRootAndV0Subcommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{"--help"}, want: []string{"Install and configure", "install", "doctor", "update", "uninstall"}},
		{name: "install", args: []string{"install", "--help"}, want: []string{"Install Matty-managed", "--dry-run"}},
		{name: "doctor", args: []string{"doctor", "--help"}, want: []string{"Check Matty setup"}},
		{name: "update", args: []string{"update", "--help"}, want: []string{"Refresh Matty-managed"}},
		{name: "uninstall", args: []string{"uninstall", "--help"}, want: []string{"Remove only Matty-managed"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, _ := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), tt.args...)
			if err != nil {
				t.Fatalf("help command failed: %v\n%s", err, out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("help output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestCommandsResolvePathsFromInjectedEnvironment(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "custom-xdg")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": xdg}, Runner: &fakeRunner{}}

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}

	wants := []string{
		"HOME=" + home,
		"CONFIG_HOME=" + xdg,
		"MATTY_STATE=" + filepath.Join(home, ".matty", "config.json"),
		"AGENT_SKILLS=" + filepath.Join(home, ".agents", "skills"),
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestCommandsAcceptFakeRunnerWithoutExecutingExternalCommands(t *testing.T) {
	tests := [][]string{
		{"install", "--dry-run"},
		{"install"},
		{"doctor"},
		{"update"},
		{"uninstall"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			opts, runner, _ := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("expected scaffold command not to execute external tools, got %#v", runner.calls)
			}
		})
	}
}

func TestCommandsDoNotCreateFilesInSandboxHome(t *testing.T) {
	tests := [][]string{
		{"install", "--dry-run"},
		{"install"},
		{"doctor"},
		{"update"},
		{"uninstall"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			opts, _, home := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			for _, path := range []string{filepath.Join(home, ".matty"), filepath.Join(home, ".agents")} {
				if t.Failed() {
					return
				}
				if exists(path) {
					t.Fatalf("command %q unexpectedly created %s", strings.Join(args, " "), path)
				}
			}
		})
	}
}

func TestResolvePathsRejectsMissingHome(t *testing.T) {
	_, err := ResolvePaths(MapEnv{})
	if err == nil {
		t.Fatal("expected missing HOME error")
	}
}

func TestResolvePathsFallsBackForRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	paths, err := ResolvePaths(MapEnv{"HOME": home, "XDG_CONFIG_HOME": "relative"})
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	want := filepath.Join(home, ".config")
	if paths.ConfigHome != want {
		t.Fatalf("ConfigHome = %q, want %q", paths.ConfigHome, want)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
