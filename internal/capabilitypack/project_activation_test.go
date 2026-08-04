package capabilitypack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectActivationIdentityIsCanonicalAndCheckoutLocal(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	if err := os.MkdirAll(filepath.Join(first, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "first-link")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "packy-home")
	canonical, err := projectActivationDirectory(home, first)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := projectActivationDirectory(home, filepath.Join(link, "nested", ".."))
	if err != nil {
		t.Fatal(err)
	}
	separate, err := projectActivationDirectory(home, second)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != equivalent {
		t.Fatalf("equivalent checkout paths have different identities: %q != %q", canonical, equivalent)
	}
	if canonical == separate {
		t.Fatal("separate checkouts shared personal activation identity")
	}

	moved := filepath.Join(root, "moved")
	if err := os.Rename(first, moved); err != nil {
		t.Fatal(err)
	}
	movedIdentity, err := projectActivationDirectory(home, moved)
	if err != nil {
		t.Fatal(err)
	}
	if movedIdentity == canonical {
		t.Fatal("moved checkout retained personal activation identity")
	}
}
