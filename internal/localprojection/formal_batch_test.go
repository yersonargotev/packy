//go:build formalverification

package localprojection

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFormalProjectionBatch replays the bounded batch model against the real
// executor. It assumes no external writes and successful rollback operations.
func TestFormalProjectionBatch(t *testing.T) {
	type fault struct {
		kind   string
		target int
	}
	faults := []fault{{"none", -1}, {"backup", 0}, {"backup", 1}, {"publish", 0}, {"publish", 1}}
	for present := 0; present < 4; present++ {
		for replace := 0; replace < 4; replace++ {
			for stale := 0; stale < 4; stale++ {
				for _, f := range faults {
					name := fmt.Sprintf("present-%d_replace-%d_stale-%d_fault-%s", present, replace, stale, f.kind)
					if f.target >= 0 {
						name += fmt.Sprintf("-%d", f.target+1)
					}
					t.Run(name, func(t *testing.T) {
						root := t.TempDir()
						targets := []string{filepath.Join(root, "target-1"), filepath.Join(root, "target-2")}
						changes := make([]FilesystemChange, 2)
						stages := make([]string, 2)
						var trace []string
						for i, target := range targets {
							old := fmt.Sprintf("baseline-%d", i+1)
							if present&(1<<i) != 0 {
								if err := os.WriteFile(target, []byte(old), 0o600); err != nil {
									t.Fatal(err)
								}
							}
							expected := "missing"
							if present&(1<<i) != 0 {
								expected = FingerprintBytes([]byte(old))
							}
							if stale&(1<<i) != 0 {
								expected = "stale-precondition"
							}
							id := fmt.Sprintf("target-%d", i+1)
							changes[i] = FilesystemChange{ID: id, Target: target, ExpectedTargetFingerprint: expected}
							if replace&(1<<i) != 0 {
								changes[i].RegularFile = true
								changes[i].FileContent = []byte(fmt.Sprintf("replacement-%d", i+1))
							} else {
								changes[i].Delete = true
							}
							stages[i] = filepath.Join(root, ".packy-batch-stage-"+FingerprintBytes([]byte(id + "\x00" + target))[:12])
							trace = append(trace, fmt.Sprintf("target=%s baseline-present=%t operation=%s stale=%t", target, present&(1<<i) != 0, map[bool]string{true: "replace", false: "delete"}[replace&(1<<i) != 0], stale&(1<<i) != 0))
						}

						// Derive the first reachable error boundary independently of the hook.
						wantError := ""
						for i := range targets {
							if stale&(1<<i) != 0 {
								wantError = "stale"
								break
							}
							if f.target == i && ((f.kind == "backup" && present&(1<<i) != 0) || (f.kind == "publish" && replace&(1<<i) != 0)) {
								wantError = "injected"
								break
							}
						}
						injected := errors.New("injected one-shot rename failure")
						used := false
						originalRename := renameTreePath
						t.Cleanup(func() { renameTreePath = originalRename })
						renameTreePath = func(oldPath, newPath string) error {
							operation := "rollback"
							for i, target := range targets {
								kind := ""
								if oldPath == target && newPath == stages[i]+".backup" {
									kind = "backup"
								} else if oldPath == stages[i] && newPath == target {
									kind = "publish"
								}
								if kind != "" {
									operation = kind
									if !used && f.target == i && f.kind == kind {
										used = true
										trace = append(trace, fmt.Sprintf("%s %s -> %s: injected failure", kind, oldPath, newPath))
										return injected
									}
									break
								}
							}
							err := originalRename(oldPath, newPath)
							trace = append(trace, fmt.Sprintf("%s %s -> %s: %v", operation, oldPath, newPath, err))
							return err
						}
						err := ApplyFilesystemChanges(changes)
						trace = append(trace, fmt.Sprintf("ApplyFilesystemChanges returned: %v", err))
						if (wantError == "" && err != nil) || (wantError == "injected" && !errors.Is(err, injected)) || (wantError == "stale" && (err == nil || !strings.Contains(err.Error(), "target changed after preflight"))) || used != (wantError == "injected") {
							t.Errorf("error=%v, want %q; fault used=%t\ntrace:\n%s", err, wantError, used, strings.Join(trace, "\n"))
						}
						for i, target := range targets {
							wantPresent := present&(1<<i) != 0
							wantContent := fmt.Sprintf("baseline-%d", i+1)
							if err == nil {
								wantPresent = replace&(1<<i) != 0
								wantContent = fmt.Sprintf("replacement-%d", i+1)
							}
							got, readErr := os.ReadFile(target)
							if (wantPresent && (readErr != nil || string(got) != wantContent)) || (!wantPresent && !os.IsNotExist(readErr)) {
								label := "unexpected successful target state:"
								if err != nil {
									label = "INVARIANT RejectedBatchRestoresTargets:"
								}
								t.Errorf("%s target=%s after error=%v: got bytes=%q read error=%v, want present=%t bytes=%q\ntrace:\n%s", label, target, err, got, readErr, wantPresent, wantContent, strings.Join(trace, "\n"))
							}
						}
					})
				}
			}
		}
	}
}
