package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// attachment.go implements the client side of the Shrine storage-attachment
// API (POST /storage/attachments).
//
// Wire contract, as implemented by AttachmentController.uploadAttachments:
//
//	POST /storage/attachments        Content-Type: multipart/form-data
//	  part "metadata" (exactly one)  Content-Type: application/json
//	                                 a JSON ARRAY of
//	                                 {fileName, entityType, entityId}
//	  part "files"    (repeated)     one per file, same ORDER as metadata
//
// The backend validator compares each uploaded part's filename against the
// matching metadata entry's fileName and rejects a mismatch, so the two lists
// must be built from the same source in the same order. A 201 returns the
// CustomResponse envelope wrapping a List<AttachmentResponse>.
//
// The pre-flight duplicate check has a DIFFERENT shape, per
// AttachmentController.checkAttachmentExists:
//
//	POST /storage/attachments/exists Content-Type: multipart/form-data
//	  part "metadata"                a single JSON OBJECT (not an array) of
//	                                 {fileName, entityType}
//	  part "file"                    singular, exactly one
//
// AttachmentExistsCheckRequest carries no entityId field at all, which matches
// the predicate it serves: AttachmentRepository.existsByEntityTypeAndFileHash
// keys on (entityType, hash) only. A 200 returns the CustomResponse envelope
// wrapping {exists: bool}.
const (
	// StorageEntityTypeTicket is the StorageEntityType enum value for tickets.
	StorageEntityTypeTicket = "TICKET"

	// attachmentsPath is the upload/list endpoint on the Village API.
	attachmentsPath = "/storage/attachments"

	// attachmentsExistsPath is the pre-flight duplicate-check endpoint.
	attachmentsExistsPath = "/storage/attachments/exists"

	// MaxAttachmentBytes mirrors the server's spring.servlet.multipart
	// max-file-size AND max-request-size (both 12MB). It is therefore both the
	// per-file cap and the cap on the combined size of one upload batch.
	// Checking it client-side turns an opaque server-side 413 into an
	// actionable message before the ticket is ever created.
	MaxAttachmentBytes int64 = 12 * 1024 * 1024

	// uploadTimeout is the deadline for attachment requests, which move up to
	// MaxAttachmentBytes and can outlast the shared 30s client default on a slow
	// link. Passing it through requestBody.timeout is what makes it effective:
	// KaizenClient.requestClient clears the shared client's own Timeout for
	// these requests, because the two deadlines do not compose (the earlier one
	// always wins, so a bare context deadline this long would be inert).
	uploadTimeout = 2 * time.Minute

	// defaultAttachmentContentType is used when neither the file extension nor
	// content sniffing identifies a type.
	defaultAttachmentContentType = "application/octet-stream"
)

// Attachment mirrors AttachmentResponse from the storage domain.
type Attachment struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	FileSize    int64  `json:"fileSize"`
	EntityType  string `json:"entityType"`
	EntityID    string `json:"entityId"`
	CreatedAt   string `json:"createdAt"`
}

// attachmentCreateMetadata mirrors AttachmentCreateRequest. All three fields
// are @NotNull server-side.
type attachmentCreateMetadata struct {
	FileName   string `json:"fileName"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
}

// ErrAttachmentUnreadable marks a failure to build a request body out of a
// local file -- the file vanished, lost its permissions, or its mount went away
// between validation and dispatch.
//
// It exists so callers can tell a LOCAL certainty apart from an UNKNOWN answer
// from the server. The two demand opposite handling: a server-side error means
// "could not determine", which is safe to continue past, whereas a local read
// failure guarantees the subsequent upload fails on the same file, so
// continuing would create exactly the orphaned ticket the pre-flight exists to
// prevent.
var ErrAttachmentUnreadable = errors.New("attachment could not be read from disk")

// attachmentExistsMetadata mirrors AttachmentExistsCheckRequest. It has no
// entityId: the server-side predicate keys on (entityType, contentHash) only,
// so the parent entity genuinely does not narrow the check.
type attachmentExistsMetadata struct {
	FileName   string `json:"fileName"`
	EntityType string `json:"entityType"`
}

// attachmentExistsResult mirrors AttachmentExistsResponse.
type attachmentExistsResult struct {
	Exists bool `json:"exists"`
}

// ValidateAttachmentPaths checks every path against the constraints the
// storage endpoint enforces, so a bad path can be rejected BEFORE any
// side-effecting call (such as creating the parent ticket) is made.
//
// Rejects: missing paths, directories and other non-regular files, unreadable
// files, empty files (the server's ensureFileNotEmpty), files over
// MaxAttachmentBytes, a batch whose combined size exceeds the server's
// max-request-size, and any two files with identical CONTENT.
//
// The content check mirrors the server's own predicate rather than
// approximating it. AttachmentServiceImpl.uploadAttachments seeds a per-request
// Set<String> of hashes and AttachmentValidator.ensureFileNotDuplicate throws
// VAL_FILE_ALREADY_EXISTS from that in-batch set BEFORE it ever consults the
// database, so two distinct files holding identical bytes are rejected even
// though neither is stored yet. That case cannot be caught by the pre-flight
// (nothing is on the server to find) and would otherwise 400 the upload after
// the ticket had already been created. Hashing with SHA-256 matches
// AttachmentHashHelper exactly, and subsumes any path-identity check: one file
// reached through two paths or a symlink yields one digest.
func ValidateAttachmentPaths(paths []string) error {
	var total int64
	seen := make(map[string]string, len(paths))

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("attachment %q does not exist", path)
			}
			return fmt.Errorf("attachment %q cannot be read: %w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("attachment %q is a directory, not a file", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("attachment %q is not a regular file", path)
		}
		if info.Size() == 0 {
			return fmt.Errorf("attachment %q is empty; the server rejects empty files", path)
		}
		// The base name is interpolated into a multipart Content-Disposition
		// header, so a CR or LF in it would let the filename inject extra part
		// headers. Rejected outright rather than sanitised: the server compares
		// the transmitted filename against the metadata entry byte for byte, so
		// silently rewriting it would only turn this into a confusing 400.
		if name := filepath.Base(path); strings.ContainsAny(name, "\r\n") {
			return fmt.Errorf("attachment %q has a line break in its file name, which cannot be transmitted", path)
		}
		if info.Size() > MaxAttachmentBytes {
			return fmt.Errorf("attachment %q is %s, which exceeds the %s per-file limit",
				path, humanBytes(info.Size()), humanBytes(MaxAttachmentBytes))
		}

		// Hashing doubles as the readability probe: stat alone does not prove the
		// bytes can be read (permissions, broken mount), and discovering that
		// during the upload would leave a ticket already created.
		digest, err := hashFile(path)
		if err != nil {
			return err
		}
		if previous, duplicate := seen[digest]; duplicate {
			// Distinguish the same path repeated from two genuinely different
			// files that happen to hold identical bytes; naming one path twice
			// reads like a bug in the message rather than a fact about the input.
			if previous == path {
				return fmt.Errorf("attachment %q is listed more than once; the server rejects duplicate content in one upload", path)
			}
			return fmt.Errorf("attachment %q has identical content to %q; the server rejects two files with the same content in one upload", path, previous)
		}
		seen[digest] = path

		total += info.Size()
	}

	// Raw-bytes pre-check. It is deliberately not the last word: the server's
	// max-request-size measures the ENCODED body, so multipart boundaries, part
	// headers and the metadata JSON all count against the same 12MB. The exact
	// check runs in UploadAttachments once the body has been built.
	if total > MaxAttachmentBytes {
		return fmt.Errorf("attachments total %s, which exceeds the %s request limit; upload fewer files",
			humanBytes(total), humanBytes(MaxAttachmentBytes))
	}

	return nil
}

// hashFile streams the file through SHA-256 and returns the lowercase hex
// digest, matching the server's AttachmentHashHelper. Streaming rather than
// reading the whole file keeps the memory cost flat regardless of file size.
//
// Errors are wrapped with the path because this is the first point at which a
// file's bytes are actually touched, so it is where permission and I/O problems
// surface. The handle is closed on every return path, error included.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("attachment %q cannot be opened: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("attachment %q could not be read: %w", path, err)
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}

// UploadAttachments uploads paths as attachments of the given parent entity.
//
// Callers are expected to have run ValidateAttachmentPaths first; this method
// still surfaces any read error it hits rather than uploading a partial file.
// The upload is a single batched request, so it is all-or-nothing: on error no
// attachment was persisted.
func (c *KaizenClient) UploadAttachments(entityType, entityID string, paths []string) ([]Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	payload, contentType, err := buildAttachmentUpload(entityType, entityID, paths)
	if err != nil {
		return nil, err
	}

	// Exact request-size check against the server's max-request-size. The
	// raw-bytes pre-check in ValidateAttachmentPaths cannot see the multipart
	// framing (~300 bytes of boundary and headers per part, plus the metadata
	// JSON), so a batch summing to exactly the limit passes there and 413s on
	// the server. The body is fully buffered anyway, so measuring it is free.
	if int64(len(payload)) > MaxAttachmentBytes {
		return nil, fmt.Errorf("the upload encodes to %s, which exceeds the server's %s request limit; upload fewer or smaller files",
			humanBytes(int64(len(payload))), humanBytes(MaxAttachmentBytes))
	}

	body, err := c.doRequestWithRetry(http.MethodPost, attachmentsPath, rawBody(contentType, payload, uploadTimeout), true)
	if err != nil {
		return nil, err
	}

	var resp APIResponse[[]Attachment]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse attachment upload response: %w", err)
	}

	return resp.Data, nil
}

// buildAttachmentUpload encodes the multipart body and returns it together
// with the Content-Type header (which carries the generated boundary).
func buildAttachmentUpload(entityType, entityID string, paths []string) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	metadata := make([]attachmentCreateMetadata, 0, len(paths))
	for _, path := range paths {
		metadata = append(metadata, attachmentCreateMetadata{
			FileName:   filepath.Base(path),
			EntityType: entityType,
			EntityID:   entityID,
		})
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode attachment metadata: %w", err)
	}

	if err := writeJSONMetadataPart(writer, metadataJSON); err != nil {
		return nil, "", err
	}

	for i, path := range paths {
		// The part name is "files" (plural, repeated) on the upload endpoint.
		if err := writeFilePart(writer, "files", metadata[i].FileName, path); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to finalise attachment upload body: %w", err)
	}

	return buf.Bytes(), writer.FormDataContentType(), nil
}

// CheckAttachmentExists asks the server whether a file with identical content
// is already attached under this entity type, without uploading anything.
//
// This exists because the server's duplicate predicate
// (existsByEntityTypeAndFileHash) is scoped to the entity TYPE, not the entity:
// a byte-identical file attached to any other ticket in the organisation makes
// the upload fail with a 400. Since attachments can only be uploaded after
// their parent ticket exists, discovering that from the upload itself would
// leave an orphaned ticket behind. Calling this first keeps the failure ahead
// of any side effect.
func (c *KaizenClient) CheckAttachmentExists(entityType, path string) (bool, error) {
	payload, contentType, err := buildAttachmentExistsCheck(entityType, path)
	if err != nil {
		// Tagged so the caller can hard-fail on this while still failing open on
		// transport and status errors below. Everything reachable from here is a
		// local, deterministic failure, not an unknown server answer.
		return false, fmt.Errorf("%w: %w", ErrAttachmentUnreadable, err)
	}

	body, err := c.doRequestWithRetry(http.MethodPost, attachmentsExistsPath, rawBody(contentType, payload, uploadTimeout), true)
	if err != nil {
		return false, err
	}

	var resp APIResponse[attachmentExistsResult]
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, fmt.Errorf("failed to parse attachment duplicate-check response: %w", err)
	}

	return resp.Data.Exists, nil
}

// buildAttachmentExistsCheck encodes the pre-flight body. Note the two
// differences from the upload body: "metadata" is a single JSON OBJECT rather
// than an array, and the file part is named "file" (singular).
func buildAttachmentExistsCheck(entityType, path string) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	fileName := filepath.Base(path)
	metadataJSON, err := json.Marshal(attachmentExistsMetadata{
		FileName:   fileName,
		EntityType: entityType,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode duplicate-check metadata: %w", err)
	}

	if err := writeJSONMetadataPart(writer, metadataJSON); err != nil {
		return nil, "", err
	}

	if err := writeFilePart(writer, "file", fileName, path); err != nil {
		return nil, "", err
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to finalise duplicate-check body: %w", err)
	}

	return buf.Bytes(), writer.FormDataContentType(), nil
}

// writeJSONMetadataPart appends the "metadata" part carrying already-marshalled
// JSON. Shared by both endpoints so the two shapes -- an array for the upload,
// a single object for the pre-flight -- cannot drift on the framing.
//
// Spring resolves this part through its HTTP message converters, so it MUST
// declare application/json. Without that header the part defaults to
// application/octet-stream and the request is rejected with a 415.
func writeJSONMetadataPart(writer *multipart.Writer, metadataJSON []byte) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="metadata"`)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("failed to create the metadata part: %w", err)
	}
	if _, err := part.Write(metadataJSON); err != nil {
		return fmt.Errorf("failed to write the metadata part: %w", err)
	}

	return nil
}

// writeFilePart appends one file part under partName. fileName is what the
// server compares against the matching metadata entry (both validator entry
// points call ensureFileNameMatches), so callers must pass the same value they
// put in the metadata.
//
// Written by hand rather than via CreateFormFile because that helper hardcodes
// application/octet-stream, and the server persists the part's Content-Type
// verbatim onto the attachment row.
func writeFilePart(writer *multipart.Writer, partName, fileName, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read attachment %q: %w", path, err)
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
		partName, escapeMultipartQuotes(fileName)))
	header.Set("Content-Type", detectContentType(path, content))

	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("failed to create upload part for %q: %w", path, err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("failed to write upload part for %q: %w", path, err)
	}

	return nil
}

// detectContentType resolves the part's Content-Type, which the server persists
// verbatim on the attachment row. Extension first (it distinguishes text/csv
// from text/plain, which sniffing cannot), then content sniffing.
func detectContentType(path string, content []byte) string {
	if byExtension := mime.TypeByExtension(filepath.Ext(path)); byExtension != "" {
		return byExtension
	}
	if len(content) > 0 {
		limit := len(content)
		if limit > 512 {
			limit = 512
		}
		if sniffed := http.DetectContentType(content[:limit]); sniffed != "" {
			return sniffed
		}
	}
	return defaultAttachmentContentType
}

// escapeMultipartQuotes mirrors the escaping mime/multipart applies internally,
// which is not exported. Needed because the file parts are built by hand (to
// set a real Content-Type) rather than via CreateFormFile.
func escapeMultipartQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

// humanBytes renders a byte count for user-facing limit messages.
func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGT"[exp])
}
