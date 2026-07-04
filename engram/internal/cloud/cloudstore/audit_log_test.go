package cloudstore

import (
	"context"
	"testing"
	"time"
)

// ─── Postgres-gated test helper ───────────────────────────────────────────────
// openTestCloudStore is defined in project_controls_test.go.

// ─── Phase 1.1: DDL + Schema ─────────────────────────────────────────────────

// TestAuditLogMigrationIdempotent verifies that calling migrate() twice produces
// no error and no duplicate tables or indexes. REQ-400 scenario 2.
func TestAuditLogMigrationIdempotent(t *testing.T) {
	cs := openTestCloudStore(t)

	ctx := context.Background()

	// First call already ran in New(), so call again directly.
	if err := cs.migrate(ctx); err != nil {
		t.Fatalf("second migrate() call returned error: %v", err)
	}

	// Third call to be absolutely certain.
	if err := cs.migrate(ctx); err != nil {
		t.Fatalf("third migrate() call returned error: %v", err)
	}
}

// TestAuditLogMigrationCreatesTable verifies that migrate() creates the
// cloud_sync_audit_log table with all required columns and 3 indexes. REQ-400 scenario 1.
func TestAuditLogMigrationCreatesTable(t *testing.T) {
	cs := openTestCloudStore(t)

	// Verify the table exists and has the expected columns.
	columns, err := queryAuditLogColumns(cs)
	if err != nil {
		t.Fatalf("querying audit log columns: %v", err)
	}

	required := []string{
		"id", "occurred_at", "contributor", "project",
		"action", "outcome", "entry_count", "reason_code", "metadata",
	}
	for _, col := range required {
		if !containsString(columns, col) {
			t.Errorf("expected column %q in cloud_sync_audit_log, got columns: %v", col, columns)
		}
	}

	// Verify the 3 required indexes exist.
	indexes, err := queryAuditLogIndexes(cs)
	if err != nil {
		t.Fatalf("querying audit log indexes: %v", err)
	}
	if len(indexes) < 3 {
		t.Errorf("expected at least 3 indexes on cloud_sync_audit_log, got %d: %v", len(indexes), indexes)
	}
}

func queryAuditLogColumns(cs *CloudStore) ([]string, error) {
	rows, err := cs.db.QueryContext(context.Background(), `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'cloud_sync_audit_log'
		ORDER BY ordinal_position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func queryAuditLogIndexes(cs *CloudStore) ([]string, error) {
	rows, err := cs.db.QueryContext(context.Background(), `
		SELECT indexname
		FROM pg_indexes
		WHERE tablename = 'cloud_sync_audit_log'
		ORDER BY indexname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var idxs []string
	for rows.Next() {
		var idx string
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		idxs = append(idxs, idx)
	}
	return idxs, rows.Err()
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// ─── Phase 1.3: InsertAuditEntry ─────────────────────────────────────────────

// TestInsertAuditEntryRoundTrip verifies that InsertAuditEntry inserts one row
// and ListAuditEntriesPaginated retrieves it with matching fields. REQ-401, REQ-402.
func TestInsertAuditEntryRoundTrip(t *testing.T) {
	cs := openTestCloudStore(t)

	entry := AuditEntry{
		Contributor: "alice",
		Project:     "test-project-audit",
		Action:      AuditActionMutationPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
		EntryCount:  5,
		ReasonCode:  "sync-paused",
	}

	if err := cs.InsertAuditEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertAuditEntry: %v", err)
	}

	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project: "test-project-audit",
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected at least 1 row, got total=%d", total)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least 1 row returned, got 0")
	}

	// Verify the first (most recent) row matches the inserted entry.
	row := rows[0]
	if row.Contributor != entry.Contributor {
		t.Errorf("contributor: got %q, want %q", row.Contributor, entry.Contributor)
	}
	if row.Project != entry.Project {
		t.Errorf("project: got %q, want %q", row.Project, entry.Project)
	}
	if row.Action != entry.Action {
		t.Errorf("action: got %q, want %q", row.Action, entry.Action)
	}
	if row.Outcome != entry.Outcome {
		t.Errorf("outcome: got %q, want %q", row.Outcome, entry.Outcome)
	}
	if row.EntryCount != entry.EntryCount {
		t.Errorf("entry_count: got %d, want %d", row.EntryCount, entry.EntryCount)
	}
	if row.ReasonCode != entry.ReasonCode {
		t.Errorf("reason_code: got %q, want %q", row.ReasonCode, entry.ReasonCode)
	}
}

// TestInsertAuditEntryCancelledContext verifies that a cancelled context returns
// an error and no row is inserted. REQ-402 scenario 2.
func TestInsertAuditEntryCancelledContext(t *testing.T) {
	cs := openTestCloudStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	entry := AuditEntry{
		Contributor: "bob",
		Project:     "test-project-cancelled",
		Action:      AuditActionChunkPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
	}

	err := cs.InsertAuditEntry(ctx, entry)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}

	// Verify no row was inserted.
	rows, total, listErr := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project: "test-project-cancelled",
	}, 10, 0)
	if listErr != nil {
		t.Fatalf("ListAuditEntriesPaginated: %v", listErr)
	}
	if total != 0 || len(rows) != 0 {
		t.Errorf("expected 0 rows after cancelled context insert, got total=%d rows=%d", total, len(rows))
	}
}

// ─── Phase 1.4: ListAuditEntriesPaginated ────────────────────────────────────

// TestAuditListPaginationAndTotal seeds 25 rows and verifies page 1 of 10
// returns 10 rows with total=25 and rows are sorted DESC. REQ-403 scenario 1.
func TestAuditListPaginationAndTotal(t *testing.T) {
	cs := openTestCloudStore(t)

	project := "test-project-pagination-" + time.Now().Format("150405")
	for i := 0; i < 25; i++ {
		if err := cs.InsertAuditEntry(context.Background(), AuditEntry{
			Contributor: "tester",
			Project:     project,
			Action:      AuditActionMutationPush,
			Outcome:     AuditOutcomeRejectedProjectPaused,
			EntryCount:  i + 1,
		}); err != nil {
			t.Fatalf("InsertAuditEntry %d: %v", i, err)
		}
	}

	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{Project: project}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated: %v", err)
	}
	if total != 25 {
		t.Errorf("expected total=25, got %d", total)
	}
	if len(rows) != 10 {
		t.Errorf("expected 10 rows on page 1, got %d", len(rows))
	}

	// Verify DESC order: each OccurredAt should be >= next one.
	for i := 1; i < len(rows); i++ {
		tPrev, errP := time.Parse(time.RFC3339, rows[i-1].OccurredAt)
		tCurr, errC := time.Parse(time.RFC3339, rows[i].OccurredAt)
		if errP != nil || errC != nil {
			continue
		}
		if tPrev.Before(tCurr) {
			t.Errorf("rows not sorted DESC: row[%d] %s is before row[%d] %s",
				i-1, rows[i-1].OccurredAt, i, rows[i].OccurredAt)
		}
	}
}

// TestAuditListContributorFilter seeds alice+bob rows and verifies contributor
// filter narrows to alice only. REQ-403 scenario 2.
func TestAuditListContributorFilter(t *testing.T) {
	cs := openTestCloudStore(t)

	project := "test-project-contrib-" + time.Now().Format("150405")
	insertN := func(contributor string, n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			if err := cs.InsertAuditEntry(context.Background(), AuditEntry{
				Contributor: contributor,
				Project:     project,
				Action:      AuditActionMutationPush,
				Outcome:     AuditOutcomeRejectedProjectPaused,
			}); err != nil {
				t.Fatalf("InsertAuditEntry: %v", err)
			}
		}
	}

	insertN("alice", 3)
	insertN("bob", 2)

	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project:     project,
		Contributor: "alice",
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total=3 for alice filter, got %d", total)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows for alice, got %d", len(rows))
	}
	for _, row := range rows {
		if row.Contributor != "alice" {
			t.Errorf("expected contributor=alice, got %q", row.Contributor)
		}
	}
}

// TestAuditListOutcomeFilter verifies outcome filter narrows to matching rows.
// REQ-403 (implied by REQ-410).
func TestAuditListOutcomeFilter(t *testing.T) {
	cs := openTestCloudStore(t)

	project := "test-project-outcome-" + time.Now().Format("150405")
	if err := cs.InsertAuditEntry(context.Background(), AuditEntry{
		Contributor: "tester",
		Project:     project,
		Action:      AuditActionMutationPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
	}); err != nil {
		t.Fatalf("InsertAuditEntry paused: %v", err)
	}
	// Insert a row with a different outcome (directly to simulate future outcomes).
	_, err := cs.db.ExecContext(context.Background(), `
		INSERT INTO cloud_sync_audit_log (contributor, project, action, outcome)
		VALUES ($1, $2, $3, $4)`,
		"tester", project, AuditActionMutationPush, "some_other_outcome",
	)
	if err != nil {
		t.Fatalf("direct insert: %v", err)
	}

	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project: project,
		Outcome: AuditOutcomeRejectedProjectPaused,
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1 for outcome filter, got %d", total)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row for outcome filter, got %d", len(rows))
	}
	if len(rows) > 0 && rows[0].Outcome != AuditOutcomeRejectedProjectPaused {
		t.Errorf("expected outcome=%q, got %q", AuditOutcomeRejectedProjectPaused, rows[0].Outcome)
	}
}

// TestAuditListTimeRangeFilter verifies that OccurredAtFrom/OccurredAtTo are
// applied as inclusive bounds. REQ-403 scenario 3.
func TestAuditListTimeRangeFilter(t *testing.T) {
	cs := openTestCloudStore(t)

	project := "test-project-timerange-" + time.Now().Format("150405")

	// Insert a row, then record timestamps bracketing it.
	before := time.Now().UTC().Add(-time.Second)
	if err := cs.InsertAuditEntry(context.Background(), AuditEntry{
		Contributor: "tester",
		Project:     project,
		Action:      AuditActionMutationPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
	}); err != nil {
		t.Fatalf("InsertAuditEntry: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	// Filter within the window.
	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project:        project,
		OccurredAtFrom: before,
		OccurredAtTo:   after,
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated with time range: %v", err)
	}
	if total < 1 {
		t.Errorf("expected at least 1 row within time range, got total=%d", total)
	}
	if len(rows) == 0 {
		t.Errorf("expected at least 1 row returned, got 0")
	}

	// Filter outside the window (future only).
	future := time.Now().UTC().Add(time.Hour)
	rows2, total2, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project:        project,
		OccurredAtFrom: future,
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated outside range: %v", err)
	}
	if total2 != 0 || len(rows2) != 0 {
		t.Errorf("expected 0 rows outside time range, got total=%d rows=%d", total2, len(rows2))
	}
}

// TestAuditListEmptyResult verifies that a filter matching no rows returns
// empty slice, total=0, err=nil. REQ-403 scenario 4.
func TestAuditListEmptyResult(t *testing.T) {
	cs := openTestCloudStore(t)

	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project: "nonexistent-project-xyz-audit-999",
	}, 10, 0)
	if err != nil {
		t.Fatalf("expected no error for empty result, got: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty rows slice, got %d rows", len(rows))
	}
}

// ─── N5: empty Metadata map stored as NULL, not {} ───────────────────────────

// TestListAuditEntriesPaginatedReturnsMetadata verifies N6: metadata inserted via
// InsertAuditEntry is returned in DashboardAuditRow.Metadata from ListAuditEntriesPaginated.
// Postgres-gated; skips when CLOUDSTORE_TEST_DSN is absent.
func TestListAuditEntriesPaginatedReturnsMetadata(t *testing.T) {
	cs := openTestCloudStore(t)

	want := map[string]any{"source": "test", "batch": float64(7)} // JSON unmarshal produces float64
	entry := AuditEntry{
		Contributor: "n6-meta-user",
		Project:     "n6-meta-proj",
		Action:      AuditActionMutationPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
		EntryCount:  1,
		Metadata:    want,
	}
	if err := cs.InsertAuditEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertAuditEntry: %v", err)
	}

	rows, total, err := cs.ListAuditEntriesPaginated(context.Background(), AuditFilter{
		Project: "n6-meta-proj",
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEntriesPaginated: %v", err)
	}
	if total < 1 || len(rows) == 0 {
		t.Fatalf("expected at least 1 row, got total=%d rows=%d", total, len(rows))
	}
	got := rows[0].Metadata
	if got == nil {
		t.Fatal("expected non-nil Metadata in DashboardAuditRow, got nil")
	}
	if got["source"] != want["source"] {
		t.Errorf("Metadata[source]: got %v, want %v", got["source"], want["source"])
	}
	if got["batch"] != want["batch"] {
		t.Errorf("Metadata[batch]: got %v, want %v", got["batch"], want["batch"])
	}
}

// TestInsertAuditEntryEmptyMetadataStoredAsNull verifies N5: when Metadata is
// a non-nil empty map (map[string]any{}), the stored value must be NULL, not {}.
// Postgres-gated; skips when CLOUDSTORE_TEST_DSN is absent.
func TestInsertAuditEntryEmptyMetadataStoredAsNull(t *testing.T) {
	cs := openTestCloudStore(t)

	entry := AuditEntry{
		Contributor: "n5-test-user",
		Project:     "n5-empty-meta-proj",
		Action:      AuditActionMutationPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
		Metadata:    map[string]any{}, // non-nil but empty — must store NULL
	}
	if err := cs.InsertAuditEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertAuditEntry with empty metadata: %v", err)
	}

	// Read back from DB — must be NULL (nil byte slice after scan).
	var metaJSON []byte
	err := cs.db.QueryRowContext(context.Background(),
		`SELECT metadata FROM cloud_sync_audit_log WHERE contributor=$1 AND project=$2 ORDER BY id DESC LIMIT 1`,
		"n5-test-user", "n5-empty-meta-proj",
	).Scan(&metaJSON)
	if err != nil {
		t.Fatalf("querying metadata: %v", err)
	}
	if metaJSON != nil {
		t.Errorf("expected NULL metadata for empty map, got %q", string(metaJSON))
	}
}

// ─── JW5: metadata field persisted in INSERT ──────────────────────────────────

// TestInsertAuditEntryPersistsMetadata verifies JW5: an AuditEntry with a
// non-nil Metadata map must have that metadata stored in the DB (not silently dropped).
// Postgres-gated; skips when CLOUDSTORE_TEST_DSN is absent.
func TestInsertAuditEntryPersistsMetadata(t *testing.T) {
	cs := openTestCloudStore(t)

	entry := AuditEntry{
		Contributor: "test-meta-user",
		Project:     "meta-proj-jw5",
		Action:      AuditActionMutationPush,
		Outcome:     AuditOutcomeRejectedProjectPaused,
		EntryCount:  3,
		ReasonCode:  "sync-paused",
		Metadata:    map[string]any{"key": "value", "count": 42},
	}
	if err := cs.InsertAuditEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertAuditEntry with metadata: %v", err)
	}

	// Verify the metadata is persisted by reading it back from the DB directly.
	var metaJSON []byte
	err := cs.db.QueryRowContext(context.Background(),
		`SELECT metadata FROM cloud_sync_audit_log WHERE contributor=$1 AND project=$2 ORDER BY id DESC LIMIT 1`,
		"test-meta-user", "meta-proj-jw5",
	).Scan(&metaJSON)
	if err != nil {
		t.Fatalf("querying metadata: %v", err)
	}
	if metaJSON == nil {
		t.Fatal("expected non-NULL metadata in DB, got NULL")
	}
	if len(metaJSON) == 0 {
		t.Fatal("expected non-empty metadata JSON in DB")
	}
}
