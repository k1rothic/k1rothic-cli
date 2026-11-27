// Package apiutil holds the HTTP plumbing shared by the provider adapters:
// a streaming-friendly client, JSON POST with bounded retries, vendor error
// decoding and the event-channel runner.
package apiutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
)

// Client has no overall timeout (responses stream for minutes) but bounds
// connection setup and the wait for response headers.
var Client = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 3 * time.Minute,
	IdleConnTimeout:       90 * time.Second,
	MaxIdleConnsPerHost:   4,
	ForceAttemptHTTP2:     true,
}}

// UserAgent is sent on every request. It identifies the client only.
var UserAgent = "k1rothic-cli"

// MaxRetries is how often a request is retried on 408/409/429/5xx or a
// connection error before any response body is consumed.
var MaxRetries = 2

// APIError is a non-2xx response from a provider.
type APIError struct {
	Provider string
	Status   int
	Message  string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("%s: %s (HTTP %d) — check the API key with /provider", e.Provider, msg, e.Status)
	}
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Provider, msg, e.Status)
}

func retryable(status int) bool {
	return status == 408 || status == 409 || status == 429 || status >= 500
}

// Do sends method url with an optional JSON body and returns the response
// when it is 2xx. Other statuses become *APIError. Headers must already
// contain authentication; they are never logged or included in errors.
func Do(ctx context.Context, name, method, url string, headers map[string]string, body any) (*http.Response, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
	}
	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rd)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("User-Agent", UserAgent)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := Client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("%s: %w", name, err)
			if !wait(ctx, backoff(attempt, "")) {
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode/100 == 2 {
			return resp, nil
		}
		apiErr := decodeError(name, resp)
		lastErr = apiErr
		if !retryable(resp.StatusCode) || resp.Header.Get("x-should-retry") == "false" || attempt == MaxRetries {
			break
		}
		if !wait(ctx, backoff(attempt, resp.Header.Get("Retry-After"))) {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func backoff(attempt int, retryAfter string) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s >= 0 {
		return min(time.Duration(s)*time.Second, 30*time.Second)
	}
	return time.Duration(500*(1<<attempt)) * time.Millisecond
}

func wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func decodeError(name string, resp *http.Response) *APIError {
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	return &APIError{Provider: name, Status: resp.StatusCode, Message: ErrorMessage(b)}
}

// ErrorMessage extracts a human message from a vendor error body:
// {"error":{"message":...}}, [{"error":{...}}] (Gemini), {"error":"..."} or
// {"message":...}. It falls back to the trimmed body.
func ErrorMessage(b []byte) string {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var arr []json.RawMessage
		if json.Unmarshal(b, &arr) == nil && len(arr) > 0 {
			b = arr[0]
		}
	}
	var v struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(b, &v) == nil {
		if len(v.Error) > 0 {
			var obj struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(v.Error, &obj) == nil && obj.Message != "" {
				return obj.Message
			}
			var s string
			if json.Unmarshal(v.Error, &s) == nil && s != "" {
				return s
			}
		}
		if v.Message != "" {
			return v.Message
		}
	}
	s := string(b)
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// Out sends events to the consumer, giving up when ctx is cancelled.
type Out struct {
	ctx context.Context
	ch  chan<- provider.Event
}

// Send delivers ev; it returns false if ctx was cancelled first.
func (o Out) Send(ev provider.Event) bool {
	select {
	case o.ch <- ev:
		return true
	case <-o.ctx.Done():
		return false
	}
}

// Text, Reasoning and Usage are shorthands for Send.
func (o Out) Text(s string) bool {
	return s == "" || o.Send(provider.Event{Kind: provider.EventTextDelta, Text: s})
}

func (o Out) Reasoning(s string) bool {
	return s == "" || o.Send(provider.Event{Kind: provider.EventReasoningDelta, Text: s})
}

func (o Out) Usage(u provider.Usage) bool {
	return o.Send(provider.Event{Kind: provider.EventUsage, Usage: &u})
}

// Run executes fn in a goroutine and guarantees the Provider contract:
// exactly one terminal event, then the channel is closed. A cancelled ctx is
// reported as an error wrapping ctx.Err().
func Run(ctx context.Context, fn func(out Out) (provider.StopReason, error)) <-chan provider.Event {
	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		stop, err := fn(Out{ctx: ctx, ch: ch})
		var final provider.Event
		switch {
		case ctx.Err() != nil:
			final = provider.Event{Kind: provider.EventError, Err: ctx.Err()}
		case err != nil:
			final = provider.Event{Kind: provider.EventError, Err: err}
		default:
			if stop == "" {
				stop = provider.StopEndTurn
			}
			final = provider.Event{Kind: provider.EventDone, Stop: stop}
		}
		// The consumer normally drains until close; don't hang forever if not.
		select {
		case ch <- final:
		case <-time.After(2 * time.Second):
		}
	}()
	return ch
}

// ErrNoKey is returned when a provider needs a key and has none.
var ErrNoKey = errors.New("no API key — add one with /provider")

// JoinURL joins a base URL and a path with exactly one slash.
func JoinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}
