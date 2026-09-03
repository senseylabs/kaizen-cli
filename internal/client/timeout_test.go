package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRequestClientClearsSharedTimeout is the structural half of the guard: a
// request carrying its own timeout must be dispatched on a client whose own
// Timeout is zero, or net/http arms the earlier of the two deadlines and the
// per-request one is silently inert.
func TestRequestClientClearsSharedTimeout(t *testing.T) {
	shared := &http.Client{Timeout: 30 * time.Second}
	c := &KaizenClient{httpClient: shared}

	withTimeout := c.requestClient(rawBody("application/octet-stream", []byte("x"), 2*time.Minute))
	if withTimeout.Timeout != 0 {
		t.Errorf("expected the cloned client's Timeout to be cleared, got %s", withTimeout.Timeout)
	}
	if withTimeout == shared {
		t.Error("expected a clone, not the shared client")
	}
	if shared.Timeout != 30*time.Second {
		t.Errorf("the shared client must not be mutated, got Timeout %s", shared.Timeout)
	}

	// Everything else keeps using the shared client untouched.
	if got := c.requestClient(jsonBody(nil)); got != shared {
		t.Error("a request without its own timeout must reuse the shared client")
	}
	if got := c.requestClient(nil); got != shared {
		t.Error("a nil request body must reuse the shared client")
	}
}

// TestPerRequestTimeoutOutlivesShortClientTimeout is the behavioural half: it
// fails against the previous implementation, where the 30s shared timeout beat
// the 2-minute upload deadline and a slow upload died early -- after the ticket
// had already been created.
func TestPerRequestTimeoutOutlivesShortClientTimeout(t *testing.T) {
	const serverDelay = 250 * time.Millisecond

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(serverDelay)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":null}`))
	}))
	defer server.Close()

	c := &KaizenClient{
		BaseURL: server.URL,
		// Far shorter than the server delay, standing in for the real 30s
		// default against a transfer that legitimately takes longer.
		httpClient: &http.Client{Timeout: 40 * time.Millisecond},
		tokenFunc:  func() (string, error) { return "token", nil },
	}

	if _, err := c.doRequestWithRetry(http.MethodPost, "/slow", jsonBody(map[string]string{"a": "b"}), false); err == nil {
		t.Fatal("expected the short shared client timeout to fail an ordinary request")
	}

	if _, err := c.doRequestWithRetry(http.MethodPost, "/slow", rawBody("application/octet-stream", []byte("payload"), 10*time.Second), false); err != nil {
		t.Fatalf("a request with its own longer timeout must survive the short shared timeout, got: %v", err)
	}
}

// TestPerRequestTimeoutErrorIsNotReportedAsUnreachable guards the message: a
// deadline expiring means the server answered too slowly, not that it is down.
func TestPerRequestTimeoutErrorIsNotReportedAsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	c := &KaizenClient{
		BaseURL:    server.URL,
		httpClient: &http.Client{},
		tokenFunc:  func() (string, error) { return "token", nil },
	}

	_, err := c.doRequestWithRetry(http.MethodPost, "/slow", rawBody("application/octet-stream", []byte("payload"), 30*time.Millisecond), false)
	if err == nil {
		t.Fatal("expected the per-request deadline to fail this request")
	}
	if got := err.Error(); !strings.Contains(got, "did not complete within") {
		t.Errorf("expected a timeout-specific message, got %q", got)
	}
	if got := err.Error(); strings.Contains(got, "could not connect") {
		t.Errorf("a timeout must not be reported as an unreachable API, got %q", got)
	}
}
