package client

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildAttachmentUploadWireFormat pins the multipart layout the Village
// AttachmentController requires: exactly one application/json "metadata" part
// holding a JSON array, followed by one "files" part per file whose filename
// matches the corresponding metadata entry.
func TestBuildAttachmentUploadWireFormat(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "shot.png")
	second := filepath.Join(dir, "server.log")
	writeFile(t, first, "png-bytes")
	writeFile(t, second, "log-bytes")

	entityID := "1f0a3c9e-0000-4000-8000-000000000001"
	payload, contentType, err := buildAttachmentUpload(StorageEntityTypeTicket, entityID, []string{first, second})
	if err != nil {
		t.Fatalf("buildAttachmentUpload returned an error: %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("failed to parse Content-Type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("expected multipart/form-data, got %q", mediaType)
	}

	reader := multipart.NewReader(strings.NewReader(string(payload)), params["boundary"])

	metadataPart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("failed to read the metadata part: %v", err)
	}
	if metadataPart.FormName() != "metadata" {
		t.Fatalf("expected the first part to be named metadata, got %q", metadataPart.FormName())
	}
	if got := metadataPart.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("metadata part must declare application/json, got %q", got)
	}
	metadataBody, err := io.ReadAll(metadataPart)
	if err != nil {
		t.Fatalf("failed to read the metadata body: %v", err)
	}

	var metadata []attachmentCreateMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("metadata part must be a JSON array: %v", err)
	}
	if len(metadata) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(metadata))
	}
	for i, expectedName := range []string{"shot.png", "server.log"} {
		if metadata[i].FileName != expectedName {
			t.Errorf("metadata[%d].fileName = %q, want %q", i, metadata[i].FileName, expectedName)
		}
		if metadata[i].EntityType != StorageEntityTypeTicket {
			t.Errorf("metadata[%d].entityType = %q, want %q", i, metadata[i].EntityType, StorageEntityTypeTicket)
		}
		if metadata[i].EntityID != entityID {
			t.Errorf("metadata[%d].entityId = %q, want %q", i, metadata[i].EntityID, entityID)
		}
	}

	for i, expected := range []struct{ name, content string }{
		{"shot.png", "png-bytes"},
		{"server.log", "log-bytes"},
	} {
		filePart, partErr := reader.NextPart()
		if partErr != nil {
			t.Fatalf("failed to read file part %d: %v", i, partErr)
		}
		if filePart.FormName() != "files" {
			t.Errorf("file part %d name = %q, want files", i, filePart.FormName())
		}
		// The server rejects a filename that differs from metadata[i].fileName.
		if filePart.FileName() != expected.name {
			t.Errorf("file part %d filename = %q, want %q", i, filePart.FileName(), expected.name)
		}
		if filePart.Header.Get("Content-Type") == "" {
			t.Errorf("file part %d is missing a Content-Type", i)
		}
		content, readErr := io.ReadAll(filePart)
		if readErr != nil {
			t.Fatalf("failed to read file part %d body: %v", i, readErr)
		}
		if string(content) != expected.content {
			t.Errorf("file part %d body = %q, want %q", i, content, expected.content)
		}
	}

	if _, err := reader.NextPart(); err != io.EOF {
		t.Errorf("expected exactly 3 parts, got more (err = %v)", err)
	}
}

// TestBuildAttachmentExistsCheckWireFormat pins the pre-flight body, whose
// shape differs from the upload body in two ways that are easy to get wrong:
// "metadata" is a single JSON OBJECT rather than an array, and the file part is
// named "file", not "files".
func TestBuildAttachmentExistsCheckWireFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	writeFile(t, path, "png-bytes")

	payload, contentType, err := buildAttachmentExistsCheck(StorageEntityTypeTicket, path)
	if err != nil {
		t.Fatalf("buildAttachmentExistsCheck returned an error: %v", err)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("failed to parse Content-Type %q: %v", contentType, err)
	}
	reader := multipart.NewReader(strings.NewReader(string(payload)), params["boundary"])

	metadataPart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("failed to read the metadata part: %v", err)
	}
	if metadataPart.FormName() != "metadata" {
		t.Fatalf("expected the first part to be named metadata, got %q", metadataPart.FormName())
	}
	if got := metadataPart.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("metadata part must declare application/json, got %q", got)
	}
	metadataBody, err := io.ReadAll(metadataPart)
	if err != nil {
		t.Fatalf("failed to read the metadata body: %v", err)
	}

	// A single object, not an array: unmarshalling into a slice must fail.
	var asArray []attachmentExistsMetadata
	if json.Unmarshal(metadataBody, &asArray) == nil {
		t.Errorf("exists-check metadata must be a JSON object, but it parsed as an array: %s", metadataBody)
	}

	var metadata attachmentExistsMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("exists-check metadata must be a JSON object: %v", err)
	}
	if metadata.FileName != "shot.png" {
		t.Errorf("metadata.fileName = %q, want shot.png", metadata.FileName)
	}
	if metadata.EntityType != StorageEntityTypeTicket {
		t.Errorf("metadata.entityType = %q, want %q", metadata.EntityType, StorageEntityTypeTicket)
	}
	// AttachmentExistsCheckRequest has no entityId field; sending one would be
	// noise at best. Confirm the encoder does not emit it.
	if strings.Contains(string(metadataBody), "entityId") {
		t.Errorf("exists-check metadata must not carry entityId, got %s", metadataBody)
	}

	filePart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("failed to read the file part: %v", err)
	}
	if filePart.FormName() != "file" {
		t.Errorf("exists-check file part must be named file (singular), got %q", filePart.FormName())
	}
	if filePart.FileName() != "shot.png" {
		t.Errorf("file part filename = %q, want shot.png", filePart.FileName())
	}

	if _, err := reader.NextPart(); err != io.EOF {
		t.Errorf("expected exactly 2 parts, got more (err = %v)", err)
	}
}

func TestValidateAttachmentPathsAcceptsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	writeFile(t, path, "hello")

	if err := ValidateAttachmentPaths([]string{path}); err != nil {
		t.Fatalf("expected a readable non-empty file to validate, got %v", err)
	}
}

func TestValidateAttachmentPathsRejections(t *testing.T) {
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty.txt")
	writeFile(t, empty, "")

	populated := filepath.Join(dir, "ok.txt")
	writeFile(t, populated, "content")

	// A distinct file, distinct name, byte-identical content.
	twin := filepath.Join(dir, "ok-copy.txt")
	writeFile(t, twin, "content")

	oversize := filepath.Join(dir, "big.bin")
	writeFile(t, oversize, "x")
	if err := os.Truncate(oversize, MaxAttachmentBytes+1); err != nil {
		t.Fatalf("failed to create the oversize fixture: %v", err)
	}

	tests := []struct {
		name     string
		paths    []string
		contains string
	}{
		{"missing", []string{filepath.Join(dir, "nope.txt")}, "does not exist"},
		{"directory", []string{dir}, "is a directory"},
		{"empty", []string{empty}, "is empty"},
		{"oversize", []string{oversize}, "per-file limit"},
		{"samePathTwice", []string{populated, populated}, "listed more than once"},
		// The case that motivated hashing instead of path comparison: two
		// genuinely different files holding the same bytes. The server's
		// in-batch duplicate check rejects these, and it does so AFTER the
		// ticket has been created, so catching it here is what prevents an
		// orphaned ticket.
		{"identicalContentDifferentNames", []string{populated, twin}, "identical content"},
		{"symlinkAlias", []string{populated, symlinkTo(t, populated, filepath.Join(dir, "alias.txt"))}, "identical content"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAttachmentPaths(tc.paths)
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected an error containing %q, got %q", tc.contains, err.Error())
			}
			// Duplicate errors must name BOTH paths so the user knows which two
			// files collided, not just that a collision happened.
			if tc.contains == "identical content" {
				for _, path := range tc.paths {
					if !strings.Contains(err.Error(), path) {
						t.Errorf("duplicate error must name %q, got %q", path, err.Error())
					}
				}
			}
		})
	}
}

// TestCheckAttachmentExistsSeparatesLocalFromServerFailures guards the split
// that lets preflightAttachments fail closed on a local read failure while
// still failing open on an unknown server answer. Collapsing the two would let
// an unreadable file through to create a ticket the upload then strands.
func TestCheckAttachmentExistsSeparatesLocalFromServerFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"message":"boom"}`))
	}))
	defer server.Close()

	c := &KaizenClient{
		BaseURL:    server.URL,
		httpClient: &http.Client{},
		tokenFunc:  func() (string, error) { return "token", nil },
	}

	dir := t.TempDir()

	// Local failure: the file is gone, so the upload is certain to fail too.
	_, err := c.CheckAttachmentExists(StorageEntityTypeTicket, filepath.Join(dir, "vanished.png"))
	if err == nil {
		t.Fatal("expected a missing file to produce an error")
	}
	if !errors.Is(err, ErrAttachmentUnreadable) {
		t.Errorf("a local read failure must be tagged ErrAttachmentUnreadable, got %v", err)
	}

	// Server failure on a perfectly readable file: an unknown answer, which the
	// caller is entitled to continue past.
	readable := filepath.Join(dir, "fine.png")
	writeFile(t, readable, "bytes")

	_, err = c.CheckAttachmentExists(StorageEntityTypeTicket, readable)
	if err == nil {
		t.Fatal("expected a 500 to produce an error")
	}
	if errors.Is(err, ErrAttachmentUnreadable) {
		t.Errorf("a server-side failure must NOT be tagged ErrAttachmentUnreadable, got %v", err)
	}
}

// TestHashFileMatchesServerDigest pins the digest to plain lowercase-hex
// SHA-256, which is what AttachmentHashHelper computes server-side. If these
// ever diverge the local duplicate check silently stops matching the server's.
func TestHashFileMatchesServerDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abc.txt")
	writeFile(t, path, "abc")

	digest, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile returned an error: %v", err)
	}

	// Well-known SHA-256 of "abc".
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if digest != want {
		t.Errorf("hashFile = %q, want %q", digest, want)
	}
}

// TestValidateAttachmentPathsRejectsLineBreaksInNames guards the multipart
// Content-Disposition header against filename-driven header injection.
func TestValidateAttachmentPathsRejectsLineBreaksInNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad\nname.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Skipf("this filesystem does not allow a newline in a file name: %v", err)
	}

	err := ValidateAttachmentPaths([]string{path})
	if err == nil {
		t.Fatal("expected a file name containing a newline to be rejected")
	}
	if !strings.Contains(err.Error(), "line break") {
		t.Fatalf("expected a line-break error, got %q", err.Error())
	}
}

func symlinkTo(t *testing.T, target, link string) string {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this environment does not support symlinks: %v", err)
	}
	return link
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write fixture %q: %v", path, err)
	}
}
