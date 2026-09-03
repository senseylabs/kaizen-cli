package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/auth"
)

// TokenFunc returns a valid access token or an error.
type TokenFunc func() (string, error)

// KaizenClient handles HTTP communication with the Kaizen API.
type KaizenClient struct {
	BaseURL    string
	OrgID      string
	httpClient *http.Client
	tokenFunc  TokenFunc
	debug      bool
}

// NewKaizenClient creates a new client with a token resolver function.
func NewKaizenClient(baseURL, orgID string, tokenFunc TokenFunc, debug bool) *KaizenClient {
	return &KaizenClient{
		BaseURL: baseURL,
		OrgID:   orgID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		tokenFunc: tokenFunc,
		debug:     debug,
	}
}

// NewKaizenClientWithToken creates a client with an explicit static token (used during login).
func NewKaizenClientWithToken(baseURL, orgID, token string) *KaizenClient {
	return &KaizenClient{
		BaseURL: baseURL,
		OrgID:   orgID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		tokenFunc: func() (string, error) { return token, nil },
	}
}

// requestBody describes an outgoing request payload together with the
// Content-Type header it must be sent under.
//
// newReader is a factory rather than a plain io.Reader because
// doRequestWithRetry may replay a request after a 401 refresh or a 429
// backoff, and a reader that has already been consumed cannot be reused.
// A nil newReader means the request carries no body.
type requestBody struct {
	contentType string
	newReader   func() (io.Reader, error)
	// timeout, when non-zero, is the ONLY deadline applied to this request:
	// requestClient hands back a clone of the shared http.Client with its
	// Timeout cleared, and a context deadline of this length is armed instead.
	// Both mechanisms are needed because they do not compose -- see
	// requestClient for why setting only the context is not enough. Used by
	// uploads, which move far more bytes than a normal JSON call and would
	// otherwise trip the shared 30s default.
	timeout time.Duration
}

// jsonBody wraps an arbitrary payload as an application/json request body.
// A nil payload yields a bodyless request that still advertises JSON, which
// preserves the historical behaviour for GET and DELETE.
func jsonBody(payload interface{}) *requestBody {
	body := &requestBody{contentType: "application/json"}
	if payload == nil {
		return body
	}
	body.newReader = func() (io.Reader, error) {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		return bytes.NewReader(jsonBytes), nil
	}
	return body
}

// rawBody wraps pre-encoded bytes sent under an explicit Content-Type.
func rawBody(contentType string, payload []byte, timeout time.Duration) *requestBody {
	return &requestBody{
		contentType: contentType,
		newReader:   func() (io.Reader, error) { return bytes.NewReader(payload), nil },
		timeout:     timeout,
	}
}

// Get performs an HTTP GET request and returns the raw response bytes.
func (c *KaizenClient) Get(path string) ([]byte, error) {
	return c.doRequest("GET", path, nil)
}

// Post performs an HTTP POST request with a JSON body.
func (c *KaizenClient) Post(path string, payload interface{}) ([]byte, error) {
	return c.doRequest("POST", path, payload)
}

// Put performs an HTTP PUT request with a JSON body.
func (c *KaizenClient) Put(path string, payload interface{}) ([]byte, error) {
	return c.doRequest("PUT", path, payload)
}

// Delete performs an HTTP DELETE request.
func (c *KaizenClient) Delete(path string) ([]byte, error) {
	return c.doRequest("DELETE", path, nil)
}

// requestClient returns the http.Client to dispatch reqBody with.
//
// An http.Client.Timeout and a request context deadline do NOT compose into
// "whichever the caller meant": net/http's setRequestCancel arms whichever of
// the two fires EARLIER. So a request carrying a generous 2-minute context
// deadline dispatched on a client with a 30s Timeout still dies at 30s, and the
// per-request deadline is silently inert. For requests that ask for their own
// timeout we therefore hand back a shallow clone with Timeout cleared, leaving
// the context deadline as the single effective limit. The clone is shallow on
// purpose: it shares the underlying Transport, so connection pooling and TLS
// session reuse are unaffected.
func (c *KaizenClient) requestClient(reqBody *requestBody) *http.Client {
	if reqBody == nil || reqBody.timeout <= 0 {
		return c.httpClient
	}
	clone := *c.httpClient
	clone.Timeout = 0
	return &clone
}

func (c *KaizenClient) doRequest(method, path string, payload interface{}) ([]byte, error) {
	return c.doRequestWithRetry(method, path, jsonBody(payload), true)
}

func (c *KaizenClient) doRequestWithRetry(method, path string, reqBody *requestBody, allowRetry bool) ([]byte, error) {
	url := c.BaseURL + path

	if reqBody == nil {
		reqBody = jsonBody(nil)
	}

	var bodyReader io.Reader
	if reqBody.newReader != nil {
		reader, readerErr := reqBody.newReader()
		if readerErr != nil {
			return nil, readerErr
		}
		bodyReader = reader
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Arm the per-request deadline. This is only half the job: the shared
	// client's own Timeout must also be cleared, which requestClient does
	// below. Cancel is deferred so the context is always released, even on the
	// early-return error paths between here and the dispatch.
	if reqBody.timeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), reqBody.timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	token, err := c.tokenFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", reqBody.contentType)
	if c.OrgID != "" {
		req.Header.Set("X-Organization-ID", c.OrgID)
	}

	if c.debug {
		_, _ = fmt.Fprintf(os.Stderr, "[DEBUG] %s %s\n", method, url)
		_, _ = fmt.Fprintf(os.Stderr, "[DEBUG] Authorization: Bearer <redacted>\n")
		if c.OrgID != "" {
			_, _ = fmt.Fprintf(os.Stderr, "[DEBUG] X-Organization-ID: %s\n", c.OrgID)
		}
	}

	resp, err := c.requestClient(reqBody).Do(req)
	if err != nil {
		// A per-request deadline expiring means the server was reachable and the
		// transfer simply ran long. Falling through to the generic branch below
		// would tell the user the API is down, sending them to debug a service
		// that is actually up. Checked first because os.IsTimeout also reports
		// true for a context deadline wrapped in *url.Error.
		if reqBody.timeout > 0 && errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("request to %s did not complete within %s. The connection may be too slow for a payload this size", url, reqBody.timeout)
		}
		if os.IsTimeout(err) || strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "connection refused") {
			return nil, fmt.Errorf("could not connect to %s. Check your network or if the API is running", c.BaseURL)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", url, err)
	}

	if c.debug {
		_, _ = fmt.Fprintf(os.Stderr, "[DEBUG] Response: %d (%d bytes)\n", resp.StatusCode, len(body))
	}

	// Handle 401: attempt token refresh and retry once
	if resp.StatusCode == http.StatusUnauthorized && allowRetry {
		if c.debug {
			_, _ = fmt.Fprintf(os.Stderr, "[DEBUG] Got 401, attempting token refresh...\n")
		}
		if refreshErr := c.tryRefreshToken(); refreshErr == nil {
			return c.doRequestWithRetry(method, path, reqBody, false)
		}
	}

	// Handle 429: respect Retry-After and retry once
	if resp.StatusCode == http.StatusTooManyRequests && allowRetry {
		retryAfter := resp.Header.Get("Retry-After")
		waitSeconds := 5 // default wait
		if retryAfter != "" {
			if parsed, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
				waitSeconds = parsed
			}
		}
		if c.debug {
			_, _ = fmt.Fprintf(os.Stderr, "[DEBUG] Got 429, waiting %ds...\n", waitSeconds)
		}
		time.Sleep(time.Duration(waitSeconds) * time.Second)
		return c.doRequestWithRetry(method, path, reqBody, false)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.parseError(resp.StatusCode, body)
	}

	return body, nil
}

func (c *KaizenClient) tryRefreshToken() error {
	store := auth.NewCredentialStore()
	creds, err := store.Load()
	if err != nil {
		return err
	}

	issuer := creds.IssuerURL
	if issuer == "" {
		return fmt.Errorf("no issuer URL in stored credentials")
	}

	tokenResp, err := auth.RefreshTokenDirect(issuer, creds.ClientID, creds.RefreshToken)
	if err != nil {
		return err
	}

	creds.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		creds.RefreshToken = tokenResp.RefreshToken
	}
	creds.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return store.Save(creds)
}

// NotFoundError indicates the requested resource was not found (HTTP 404).
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// ForbiddenError indicates access was denied (HTTP 403).
type ForbiddenError struct {
	Message string
}

func (e *ForbiddenError) Error() string {
	return e.Message
}

func (c *KaizenClient) parseError(statusCode int, body []byte) error {
	var apiErr APIError
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
		switch statusCode {
		case http.StatusNotFound:
			return &NotFoundError{Message: apiErr.Message}
		case http.StatusForbidden:
			return &ForbiddenError{Message: apiErr.Message}
		default:
			return &apiErr
		}
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized. Run 'kaizen login' to authenticate")
	case http.StatusForbidden:
		return &ForbiddenError{Message: "access denied. You may not have permission for this operation"}
	case http.StatusNotFound:
		return &NotFoundError{Message: "resource not found"}
	case http.StatusInternalServerError:
		return fmt.Errorf("server error. Try again later")
	default:
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		return fmt.Errorf("request failed (%d): %s", statusCode, bodyStr)
	}
}
