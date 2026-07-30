package issuedelivery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const revisionTestRunID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestRevisionStoreRoundTripAndPreservesInitialSnapshot(t *testing.T) {
	commonDir := t.TempDir()
	initial := []byte(`{"state":"needs-review"}`)
	updated := []byte(`{"state":"completed"}`)

	withLockedRevisionStore(t, commonDir, func(store lockedIssueStore) {
		if err := store.storeAndActivate(revisionTestRunID, initial); err != nil {
			t.Fatalf("store initial run: %v", err)
		}

		revision, err := store.storeRevisionAndActivate(revisionTestRunID, updated)
		if err != nil {
			t.Fatalf("store revision: %v", err)
		}
		digest := sha256.Sum256(updated)
		if want := hex.EncodeToString(digest[:]); revision != want {
			t.Fatalf("revision = %q, want %q", revision, want)
		}

		active, found, err := store.loadActiveRecord()
		if err != nil || !found {
			t.Fatalf("load active record: found=%v err=%v", found, err)
		}
		if active.RunID != revisionTestRunID || active.Revision != revision {
			t.Fatalf("active record = %#v", active)
		}
		runID, data, found, err := store.loadActive()
		if err != nil || !found {
			t.Fatalf("load active revision: found=%v err=%v", found, err)
		}
		if runID != revisionTestRunID || !bytes.Equal(data, updated) {
			t.Fatalf("active = (%q, %s), want revision bytes", runID, data)
		}
		revisionData, found, err := store.loadRevision(revisionTestRunID, revision)
		if err != nil || !found || !bytes.Equal(revisionData, updated) {
			t.Fatalf("load revision: found=%v data=%s err=%v", found, revisionData, err)
		}
		snapshot, found, err := store.loadRun(revisionTestRunID)
		if err != nil || !found || !bytes.Equal(snapshot, initial) {
			t.Fatalf("initial snapshot changed: found=%v data=%s err=%v", found, snapshot, err)
		}
	})
}

func TestRevisionStoreAdoptsIdenticalBytesAndRejectsConflict(t *testing.T) {
	commonDir := t.TempDir()
	data := []byte(`{"state":"completed"}`)

	withLockedRevisionStore(t, commonDir, func(store lockedIssueStore) {
		if err := store.storeAndActivate(revisionTestRunID, []byte(`{"state":"needs-review"}`)); err != nil {
			t.Fatalf("store initial run: %v", err)
		}
		revision, err := store.storeRevisionAndActivate(revisionTestRunID, data)
		if err != nil {
			t.Fatalf("store revision: %v", err)
		}
		if adopted, err := store.storeRevisionAndActivate(revisionTestRunID, data); err != nil || adopted != revision {
			t.Fatalf("adopt identical revision: revision=%q err=%v", adopted, err)
		}

		path := filepath.Join(commonDir, "packy", "issue-delivery", "issue-357", "revisions", revisionTestRunID, revision+".json")
		if err := os.WriteFile(path, []byte("conflict"), 0600); err != nil {
			t.Fatalf("tamper revision fixture: %v", err)
		}
		if _, err := store.storeRevisionAndActivate(revisionTestRunID, data); err == nil ||
			!strings.Contains(err.Error(), "immutable") {
			t.Fatalf("conflicting revision error = %v, want immutable", err)
		}
		if _, _, err := store.loadRevision(revisionTestRunID, revision); err == nil ||
			!strings.Contains(err.Error(), "does not match its identity") {
			t.Fatalf("tampered revision load error = %v", err)
		}
	})
}

func TestRevisionStoreLoadsBackwardCompatibleSnapshotPointer(t *testing.T) {
	commonDir := t.TempDir()
	initial := []byte(`{"state":"needs-review"}`)

	withLockedRevisionStore(t, commonDir, func(store lockedIssueStore) {
		if err := store.storeAndActivate(revisionTestRunID, initial); err != nil {
			t.Fatalf("store initial run: %v", err)
		}
		active, found, err := store.loadActiveRecord()
		if err != nil || !found || active.Revision != "" {
			t.Fatalf("load legacy active record: active=%#v found=%v err=%v", active, found, err)
		}
		_, data, found, err := store.loadActive()
		if err != nil || !found || !bytes.Equal(data, initial) {
			t.Fatalf("load legacy snapshot: found=%v data=%s err=%v", found, data, err)
		}
	})
}

func TestRevisionStoreLeavesUnactivatedRevisionAsHarmlessOrphan(t *testing.T) {
	commonDir := t.TempDir()
	initial := []byte(`{"state":"needs-review"}`)
	updated := []byte(`{"state":"completed"}`)

	withLockedRevisionStore(t, commonDir, func(store lockedIssueStore) {
		if err := store.storeAndActivate(revisionTestRunID, initial); err != nil {
			t.Fatalf("store initial run: %v", err)
		}
		revision, err := store.storeRevisionAndActivate(revisionTestRunID, updated)
		if err != nil {
			t.Fatalf("store revision: %v", err)
		}
		if err := store.activate(revisionTestRunID); err != nil {
			t.Fatalf("restore snapshot pointer: %v", err)
		}

		_, data, found, err := store.loadActive()
		if err != nil || !found || !bytes.Equal(data, initial) {
			t.Fatalf("load active snapshot: found=%v data=%s err=%v", found, data, err)
		}
		orphan, found, err := store.loadRevision(revisionTestRunID, revision)
		if err != nil || !found || !bytes.Equal(orphan, updated) {
			t.Fatalf("load orphan revision: found=%v data=%s err=%v", found, orphan, err)
		}
	})
}

func TestRevisionStoreRejectsSymlinkedRevisionDirectory(t *testing.T) {
	commonDir := t.TempDir()
	outside := t.TempDir()

	withLockedRevisionStore(t, commonDir, func(store lockedIssueStore) {
		if err := store.storeAndActivate(revisionTestRunID, []byte(`{"state":"needs-review"}`)); err != nil {
			t.Fatalf("store initial run: %v", err)
		}
		issuePath := filepath.Join(commonDir, "packy", "issue-delivery", "issue-357")
		if err := os.Symlink(outside, filepath.Join(issuePath, "revisions")); err != nil {
			t.Fatalf("create revisions symlink: %v", err)
		}
		if _, err := store.storeRevisionAndActivate(revisionTestRunID, []byte(`{"state":"completed"}`)); err == nil {
			t.Fatal("store revision through symlink succeeded")
		}
	})
}

func withLockedRevisionStore(t *testing.T, commonDir string, fn func(lockedIssueStore)) {
	t.Helper()
	err := (fileRunStore{}).withIssueLock(context.Background(), commonDir, 357, func(store lockedIssueStore) error {
		fn(store)
		return nil
	})
	if err != nil {
		t.Fatalf("with issue lock: %v", err)
	}
}
