package client

import (
	"bytes"
	"encoding/json"
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
	BaseURL      string
	OrgID        string
	httpClient   *http.Client
	tokenFunc    TokenFunc
	clientSecret string
	debug        bool
}

// NewKaizenClient creates a new client with a token resolver function.
func NewKaizenClient(baseURL, orgID, clientSecret string, tokenFunc TokenFunc, debug bool) *KaizenClient {
	return &KaizenClient{
		BaseURL:      baseURL,
		OrgID:        orgID,
		clientSecret: clientSecret,
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

func (c *KaizenClient) doRequest(method, path string, payload interface{}) ([]byte, error) {
	return c.doRequestWithRetry(method, path, payload, true)
}

func (c *KaizenClient) doRequestWithRetry(method, path string, payload interface{}, allowRetry bool) ([]byte, error) {
	url := c.BaseURL + path

	var bodyReader io.Reader
	if payload != nil {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	token, err := c.tokenFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
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
			return c.doRequestWithRetry(method, path, payload, false)
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
		return c.doRequestWithRetry(method, path, payload, false)
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

	tokenResp, err := auth.RefreshToken(issuer, creds.ClientID, c.clientSecret, creds.RefreshToken)
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
