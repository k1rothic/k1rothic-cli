package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/anthropic"
	"github.com/akutor/k1rothic-cli/internal/core/provider/apiutil"
	"github.com/akutor/k1rothic-cli/internal/core/provider/gemini"
	"github.com/akutor/k1rothic-cli/internal/core/provider/openai"
)

type captured struct {
	path    string
	headers http.Header
	body    map[string]any
}

// server replays sse for every request and records the last request.
func server(t *testing.T, status int, sse string) (*httptest.Server, *captured) {
	t.Helper()
	c := &captured{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.path = r.URL.RequestURI()
		c.headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &c.body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, sse)
	}))
	t.Cleanup(s.Close)
	return s, c
}

type result struct {
	text, reasoning string
	usage           provider.Usage
	stop            provider.StopReason
	err             error
	terminal        int
}

func collect(t *testing.T, ch <-chan provider.Event) result {
	t.Helper()
	var r result
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				if r.terminal != 1 {
					t.Fatalf("want exactly one terminal event, got %d", r.terminal)
				}
				return r
			}
			switch ev.Kind {
			case provider.EventTextDelta:
				r.text += ev.Text
			case provider.EventReasoningDelta:
				r.reasoning += ev.Text
			case provider.EventUsage:
				r.usage = *ev.Usage
			case provider.EventDone:
				r.stop = ev.Stop
				r.terminal++
			case provider.EventError:
				r.err = ev.Err
				r.terminal++
			}
		case <-timeout:
			t.Fatal("stream did not close")
		}
	}
}

var req = provider.Request{
	Model:    "m-1",
	System:   "be brief",
	Effort:   provider.EffortHigh,
	Messages: []provider.Message{{Role: provider.RoleUser, Text: "hi"}, {Role: provider.RoleAssistant, Text: "yo"}, {Role: provider.RoleUser, Text: "more"}},
}

const anthropicSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10,"cache_read_input_tokens":5,"cache_creation_input_tokens":2,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}

event: ping
data: {"type":"ping"}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hel"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"lo"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}

event: message_stop
data: {"type":"message_stop"}

`

func TestAnthropicStream(t *testing.T) {
	s, c := server(t, 200, anthropicSSE)
	r := collect(t, anthropic.New("sk-a", s.URL).Stream(context.Background(), req))
	if r.err != nil || r.text != "Hello" || r.reasoning != "hmm" || r.stop != provider.StopEndTurn {
		t.Fatalf("%+v", r)
	}
	if r.usage != (provider.Usage{InputTokens: 10, CacheReadTokens: 5, CacheWriteTokens: 2, OutputTokens: 42}) {
		t.Fatalf("usage %+v", r.usage)
	}
	if c.path != "/v1/messages" || c.headers.Get("x-api-key") != "sk-a" || c.headers.Get("anthropic-version") != anthropic.APIVersion {
		t.Fatalf("request %s %v", c.path, c.headers)
	}
	if c.body["system"] != "be brief" || c.body["stream"] != true {
		t.Fatalf("body %v", c.body)
	}
	th := c.body["thinking"].(map[string]any)
	oc := c.body["output_config"].(map[string]any)
	if th["type"] != "adaptive" || th["display"] != "summarized" || oc["effort"] != "high" {
		t.Fatalf("thinking/effort %v %v", th, oc)
	}
	if _, bad := c.body["temperature"]; bad {
		t.Fatal("temperature must not be sent")
	}
	if n := len(c.body["messages"].([]any)); n != 3 {
		t.Fatalf("messages = %d", n)
	}
}

func TestAnthropicStreamError(t *testing.T) {
	s, _ := server(t, 200, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n")
	r := collect(t, anthropic.New("k", s.URL).Stream(context.Background(), req))
	if r.err == nil || !strings.Contains(r.err.Error(), "Overloaded") {
		t.Fatalf("err = %v", r.err)
	}
}

func TestHTTPErrorIsReadableAndNotRetriedOn401(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer s.Close()
	r := collect(t, anthropic.New("bad-key", s.URL).Stream(context.Background(), req))
	var apiErr *apiutil.APIError
	if !errors.As(r.err, &apiErr) || apiErr.Status != 401 || !strings.Contains(r.err.Error(), "invalid x-api-key") {
		t.Fatalf("err = %v", r.err)
	}
	if strings.Contains(r.err.Error(), "bad-key") {
		t.Fatal("key leaked into error")
	}
	if calls != 1 {
		t.Fatalf("401 retried %d times", calls)
	}
}

func TestRetryOn529ThenSuccess(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(529)
			return
		}
		_, _ = io.WriteString(w, anthropicSSE)
	}))
	defer s.Close()
	r := collect(t, anthropic.New("k", s.URL).Stream(context.Background(), req))
	if r.err != nil || r.text != "Hello" || calls != 2 {
		t.Fatalf("calls=%d %+v", calls, r)
	}
}

func TestMissingKey(t *testing.T) {
	r := collect(t, anthropic.New("", "http://127.0.0.1:1").Stream(context.Background(), req))
	if !errors.Is(r.err, apiutil.ErrNoKey) {
		t.Fatalf("err = %v", r.err)
	}
}

func TestCancelMidStream(t *testing.T) {
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{}}}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer s.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	ch := anthropic.New("k", s.URL).Stream(ctx, req)
	<-ch // usage
	cancel()
	r := collect(t, ch)
	if !errors.Is(r.err, context.Canceled) {
		t.Fatalf("err = %v", r.err)
	}
}

const openaiSSE = `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}

data: {"choices":[{"delta":{"reasoning_content":"think"}}]}

data: {"choices":[{"delta":{"content":"Hi"}}]}

data: {"choices":[{"delta":{"content":" there"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":8},"completion_tokens_details":{"reasoning_tokens":3}}}

data: [DONE]

`

func TestOpenAIStream(t *testing.T) {
	s, c := server(t, 200, openaiSSE)
	r := collect(t, openai.New("sk-o", s.URL).Stream(context.Background(), req))
	if r.err != nil || r.text != "Hi there" || r.reasoning != "think" {
		t.Fatalf("%+v", r)
	}
	if r.usage != (provider.Usage{InputTokens: 12, CacheReadTokens: 8, OutputTokens: 7, ReasoningTokens: 3}) {
		t.Fatalf("usage %+v", r.usage)
	}
	if c.path != "/chat/completions" || c.headers.Get("Authorization") != "Bearer sk-o" {
		t.Fatalf("request %s", c.path)
	}
	msgs := c.body["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "developer" || c.body["reasoning_effort"] != "high" {
		t.Fatalf("body %v", c.body)
	}
	so := c.body["stream_options"].(map[string]any)
	if so["include_usage"] != true {
		t.Fatal("usage not requested")
	}
}

func TestOpenAICompatibleIsPermissive(t *testing.T) {
	s, c := server(t, 200, openaiSSE)
	r := collect(t, openai.NewCompatible("", s.URL+"/v1").Stream(context.Background(), req))
	if r.err != nil || r.text != "Hi there" {
		t.Fatalf("%+v", r)
	}
	if c.path != "/v1/chat/completions" || c.headers.Get("Authorization") != "" {
		t.Fatalf("path %s auth %q", c.path, c.headers.Get("Authorization"))
	}
	if _, ok := c.body["reasoning_effort"]; ok {
		t.Fatal("compatible servers get no reasoning_effort")
	}
	if c.body["messages"].([]any)[0].(map[string]any)["role"] != "system" {
		t.Fatal("compatible servers get a system role")
	}
}

func TestOpenAIEffortOffOmitsReasoning(t *testing.T) {
	s, c := server(t, 200, openaiSSE)
	r2 := req
	r2.Effort = provider.EffortOff
	collect(t, openai.New("k", s.URL).Stream(context.Background(), r2))
	if _, ok := c.body["reasoning_effort"]; ok {
		t.Fatal("effort off must omit reasoning_effort")
	}
}

const geminiSSE = `data: {"candidates":[{"content":{"parts":[{"text":"plan","thought":true}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"Bon"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"jour"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":30,"candidatesTokenCount":5,"thoughtsTokenCount":4,"cachedContentTokenCount":10}}

`

func TestGeminiStream(t *testing.T) {
	s, c := server(t, 200, geminiSSE)
	r2 := req
	r2.Model = "gemini-2.5-flash"
	r := collect(t, gemini.New("g-key", s.URL).Stream(context.Background(), r2))
	if r.err != nil || r.text != "Bonjour" || r.reasoning != "plan" {
		t.Fatalf("%+v", r)
	}
	if r.usage != (provider.Usage{InputTokens: 20, CacheReadTokens: 10, OutputTokens: 9, ReasoningTokens: 4}) {
		t.Fatalf("usage %+v", r.usage)
	}
	if c.path != "/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse" || c.headers.Get("x-goog-api-key") != "g-key" {
		t.Fatalf("request %s", c.path)
	}
	if strings.Contains(c.path, "g-key") {
		t.Fatal("key must not be in the URL")
	}
	contents := c.body["contents"].([]any)
	if len(contents) != 3 || contents[1].(map[string]any)["role"] != "model" {
		t.Fatalf("contents %v", contents)
	}
	tc := c.body["generationConfig"].(map[string]any)["thinkingConfig"].(map[string]any)
	if tc["thinkingBudget"].(float64) != 24576 || tc["includeThoughts"] != true {
		t.Fatalf("thinking %v", tc)
	}
}

func TestGeminiHTTPError(t *testing.T) {
	s, _ := server(t, 400, `[{"error":{"code":400,"message":"API key not valid","status":"INVALID_ARGUMENT"}}]`)
	r := collect(t, gemini.New("k", s.URL).Stream(context.Background(), req))
	if r.err == nil || !strings.Contains(r.err.Error(), "API key not valid") {
		t.Fatalf("err = %v", r.err)
	}
}

func TestModelListings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"claude-x","display_name":"Claude X","max_input_tokens":1000000}]}`)
	})
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"gpt-5"},{"id":"text-embedding-3-small"},{"id":"o3"},{"id":"whisper-1"}]}`)
	})
	mux.HandleFunc("/v1beta/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":[{"name":"models/gemini-x","inputTokenLimit":1048576,"supportedGenerationMethods":["generateContent"]},{"name":"models/embedding-001","supportedGenerationMethods":["embedContent"]}]}`)
	})
	s := httptest.NewServer(mux)
	defer s.Close()
	ctx := context.Background()

	am, err := anthropic.New("k", s.URL).Models(ctx)
	if err != nil || len(am) != 1 || am[0].ContextWindow != 1_000_000 {
		t.Fatalf("anthropic %v %v", am, err)
	}
	om, err := openai.New("k", s.URL).Models(ctx)
	if err != nil || len(om) != 2 {
		t.Fatalf("openai %v %v", om, err)
	}
	gm, err := gemini.New("k", s.URL).Models(ctx)
	if err != nil || len(gm) != 1 || gm[0].ID != "gemini-x" {
		t.Fatalf("gemini %v %v", gm, err)
	}
}
