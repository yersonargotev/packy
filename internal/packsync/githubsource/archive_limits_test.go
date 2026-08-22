package githubsource

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
)

func TestWithSnapshotRejectsOversizedFileAndCleans(t *testing.T) {
	archive := archiveBytes(t, []tarEntry{
		{name: "root/", mode: 0o755, kind: tar.TypeDir},
		{name: "root/large", mode: 0o644, data: []byte("too large")},
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	client.archiveLimits = archiveLimits{
		maxExpandedBytes: 1 << 20,
		maxFileBytes:     8,
		maxEntries:       10,
		maxPathDepth:     10,
	}
	temporary := t.TempDir()
	visited := false
	err := client.WithSnapshot(context.Background(), packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		visited = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "file size") {
		t.Fatalf("oversized file error = %v", err)
	}
	if visited {
		t.Fatal("visitor called for rejected archive")
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected archive was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

func TestWithSnapshotRejectsAggregateExpandedSizeAndCleans(t *testing.T) {
	archive := archiveBytes(t, []tarEntry{
		{name: "root/", mode: 0o755, kind: tar.TypeDir},
		{name: "root/one", mode: 0o644, data: []byte("123456")},
		{name: "root/two", mode: 0o644, data: []byte("abcdef")},
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	client.archiveLimits = archiveLimits{
		maxExpandedBytes: 10,
		maxFileBytes:     8,
		maxEntries:       10,
		maxPathDepth:     10,
	}
	temporary := t.TempDir()
	err := client.WithSnapshot(context.Background(), packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		t.Fatal("visitor called for rejected archive")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "expanded size") {
		t.Fatalf("aggregate expanded-size error = %v", err)
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected archive was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

func TestWithSnapshotRejectsAggregateExpandedMetadataAndCleans(t *testing.T) {
	archiveEntries := []tarEntry{{name: "root/", mode: 0o755, kind: tar.TypeDir}}
	for index := 0; index < 20; index++ {
		archiveEntries = append(archiveEntries, tarEntry{name: "root/directory-" + string(rune('a'+index)) + "/", mode: 0o755, kind: tar.TypeDir})
	}
	archive := archiveBytes(t, archiveEntries)
	const expandedLimit = 4 << 10
	if len(archive) >= expandedLimit {
		t.Fatalf("fixture is not compressed below expanded limit: compressed=%d limit=%d", len(archive), expandedLimit)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	client.archiveLimits = archiveLimits{
		maxExpandedBytes: expandedLimit,
		maxFileBytes:     1 << 20,
		maxEntries:       100,
		maxPathDepth:     10,
	}
	temporary := t.TempDir()
	err := client.WithSnapshot(context.Background(), packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		t.Fatal("visitor called for expanded metadata bomb")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "expanded size") {
		t.Fatalf("expanded metadata error = %v", err)
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected archive was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

func TestWithSnapshotRejectsTooManyArchiveEntriesAndCleans(t *testing.T) {
	archive := archiveBytes(t, []tarEntry{
		{name: "root/", mode: 0o755, kind: tar.TypeDir},
		{name: "root/one", mode: 0o644, data: []byte("1")},
		{name: "root/two", mode: 0o644, data: []byte("2")},
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	client.archiveLimits = archiveLimits{
		maxExpandedBytes: 1 << 20,
		maxFileBytes:     1 << 20,
		maxEntries:       2,
		maxPathDepth:     10,
	}
	temporary := t.TempDir()
	err := client.WithSnapshot(context.Background(), packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		t.Fatal("visitor called for rejected archive")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "entry count") {
		t.Fatalf("entry-count error = %v", err)
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected archive was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

func TestWithSnapshotRejectsDeepArchivePathAndCleans(t *testing.T) {
	archive := archiveBytes(t, []tarEntry{
		{name: "root/", mode: 0o755, kind: tar.TypeDir},
		{name: "root/one/two/three/file", mode: 0o644, data: []byte("x")},
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	client.archiveLimits = archiveLimits{
		maxExpandedBytes: 1 << 20,
		maxFileBytes:     1 << 20,
		maxEntries:       10,
		maxPathDepth:     3,
	}
	temporary := t.TempDir()
	err := client.WithSnapshot(context.Background(), packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		t.Fatal("visitor called for rejected archive")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "path depth") {
		t.Fatalf("path-depth error = %v", err)
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected archive was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

func TestWithSnapshotStopsCopyWhenContextIsCanceledAndCleans(t *testing.T) {
	archive := storedArchiveBytes(t, []tarEntry{
		{name: "root/", mode: 0o755, kind: tar.TypeDir},
		{name: "root/file", mode: 0o644, data: bytes.Repeat([]byte("x"), 64<<10)},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &cancelingReader{
		reader:    bytes.NewReader(archive),
		remaining: 2 << 10,
		cancel:    cancel,
	}
	client := newClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(body),
			Header:     make(http.Header),
		}, nil
	})}, "https://example.test")
	temporary := t.TempDir()
	visited := false
	err := client.WithSnapshot(ctx, packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		visited = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled copy error = %v", err)
	}
	if visited {
		t.Fatal("visitor called after context cancellation")
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("canceled acquisition was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

func TestWithSnapshotRejectsNegativeHeaderSizeAndCleans(t *testing.T) {
	archive := archiveWithNegativeSymlinkSize(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	temporary := t.TempDir()
	err := client.WithSnapshot(context.Background(), packsync.Candidate{Repository: "o/r", Commit: commitSHA}, temporary, func(string) error {
		t.Fatal("visitor called for archive with negative header size")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("negative header-size error = %v", err)
	}
	entries, readErr := os.ReadDir(temporary)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected archive was not cleaned: entries=%v err=%v", entries, readErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type cancelingReader struct {
	reader    *bytes.Reader
	remaining int
	cancel    context.CancelFunc
}

func (reader *cancelingReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 64 {
		buffer = buffer[:64]
	}
	read, err := reader.reader.Read(buffer)
	reader.remaining -= read
	if reader.remaining <= 0 && reader.cancel != nil {
		reader.cancel()
		reader.cancel = nil
	}
	return read, err
}

func storedArchiveBytes(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&output, gzip.NoCompression)
	if err != nil {
		t.Fatal(err)
	}
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Typeflag: entry.kind, Size: int64(len(entry.data))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func archiveWithNegativeSymlinkSize(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "root/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "root/link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "target", Size: -1}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
