// Package openai adapts the Chat Completions streaming API to the normalized
// provider interface. The same adapter serves OpenAI and any
// OpenAI-compatible endpoint (local servers, gateways); it is a hand-written
// HTTP client.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/apiutil"
	"github.com/akutor/k1rothic-cli/internal/core/provider/sse"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-5"
)

// Provider is the Chat Completions adapter.
type Provider struct {
	name    string
	BaseURL string
	Key     string
	// Compatible relaxes OpenAI-specific behaviour for third-party servers:
	// "system" instead of "developer", max_tokens instead of
	// max_completion_tokens, no reasoning_effort, optional key.
	Compatible bool
	// Responses selects the Responses API (/responses) instead of Chat
	// Completions.
	Responses bool
}

// New returns an adapter for api.openai.com (or baseURL).
func New(key, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Provider{name: "openai", BaseURL: baseURL, Key: key}
}

// NewCompatible returns a Chat Completions adapter for a custom server.
func NewCompatible(key, baseURL string) *Provider {
	return &Provider{name: "custom", BaseURL: baseURL, Key: key, Compatible: true}
}

// WithResponses switches the adapter to the Responses API.
func (p *Provider) WithResponses() *Provider { p.Responses = true; return p }

func (p *Provider) Name() string { return p.name }

func (p *Provider) headers() map[string]string {
	h := map[string]string{}
	if p.Key != "" {
		h["Authorization"] = "Bearer " + p.Key
	}
	return h
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model               string         `json:"model"`
	Messages            []message      `json:"messages"`
	Stream              bool           `json:"stream"`
	StreamOptions       map[string]any `json:"stream_options,omitempty"`
	ReasoningEffort     string         `json:"reasoning_effort,omitempty"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	MaxTokens           int            `json:"max_tokens,omitempty"`
}

func (p *Provider) buildRequest(req provider.Request) request {
	r := request{Model: req.Model, Stream: true, StreamOptions: map[string]any{"include_usage": true}}
	if req.System != "" {
		role := "developer"
		if p.Compatible {
			role = "system"
		}
		r.Messages = append(r.Messages, message{Role: role, Content: req.System})
	}
	for _, m := range req.Messages {
		if m.Text == "" {
			continue
		}
		role := "user"
		if m.Role == provider.RoleAssistant {
			role = "assistant"
		}
		r.Messages = append(r.Messages, message{Role: role, Content: m.Text})
	}
	if p.Compatible {
		r.MaxTokens = req.MaxTokens
	} else {
		r.MaxCompletionTokens = req.MaxTokens
		// Non-reasoning models reject reasoning_effort; "off" omits it.
		if req.Effort != "" && req.Effort != provider.EffortOff {
			r.ReasoningEffort = string(req.Effort)
		}
	}
	return r
}

type chunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"` // DeepSeek, vLLM, many gateways
			Reasoning        string `json:"reasoning"`         // OpenRouter, Ollama
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// Stream implements provider.Provider.
func (p *Provider) Stream(ctx context.Context, req provider.Request) <-chan provider.Event {
	return apiutil.Run(ctx, func(out apiutil.Out) (provider.StopReason, error) {
		if p.Key == "" && !p.Compatible {
			return "", apiutil.ErrNoKey
		}
		if p.BaseURL == "" {
			return "", fmt.Errorf("%s: no base_url configured (set [providers.%s] base_url in config.toml)", p.name, p.name)
		}
		if p.Responses {
			resp, err := apiutil.Do(ctx, p.name, "POST", apiutil.JoinURL(p.BaseURL, "/responses"), p.headers(), buildResponsesRequest(req))
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			return parseResponsesStream(p.name, resp.Body, out)
		}
		resp, err := apiutil.Do(ctx, p.name, "POST", apiutil.JoinURL(p.BaseURL, "/chat/completions"), p.headers(), p.buildRequest(req))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		return parseStream(p.name, resp.Body, out)
	})
}

func parseStream(name string, body io.Reader, out apiutil.Out) (provider.StopReason, error) {
	rd := sse.NewReader(body)
	stop := provider.StopEndTurn
	sawData := false
	for {
		ev, err := rd.Next()
		if errors.Is(err, io.EOF) {
			if !sawData {
				return "", fmt.Errorf("%s: empty response", name)
			}
			return stop, nil
		}
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		data := strings.TrimSpace(ev.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return stop, nil
		}
		sawData = true
		var c chunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			continue
		}
		if len(c.Error) > 0 && string(c.Error) != "null" {
			return "", fmt.Errorf("%s: %s", name, apiutil.ErrorMessage([]byte(`{"error":`+string(c.Error)+`}`)))
		}
		for _, ch := range c.Choices {
			r := ch.Delta.ReasoningContent
			if r == "" {
				r = ch.Delta.Reasoning
			}
			if !out.Reasoning(r) || !out.Text(ch.Delta.Content) {
				return "", nil
			}
			switch ch.FinishReason {
			case "length":
				stop = provider.StopMaxTokens
			case "tool_calls":
				stop = provider.StopToolUse
			case "content_filter":
				stop = provider.StopRefusal
			}
		}
		if c.Usage != nil {
			u := c.Usage
			cached := u.PromptTokensDetails.CachedTokens
			if !out.Usage(provider.Usage{
				InputTokens:     u.PromptTokens - cached,
				CacheReadTokens: cached,
				OutputTokens:    u.CompletionTokens,
				ReasoningTokens: u.CompletionTokensDetails.ReasoningTokens,
			}) {
				return "", nil
			}
		}
	}
}

// chatModel filters api.openai.com's model list down to chat models.
func chatModel(id string) bool {
	for _, bad := range []string{"embedding", "tts", "whisper", "dall-e", "image", "audio", "realtime", "transcribe", "moderation", "search", "davinci", "babbage", "codex-mini", "computer-use"} {
		if strings.Contains(id, bad) {
			return false
		}
	}
	if strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "chatgpt-") {
		return true
	}
	return len(id) > 1 && id[0] == 'o' && id[1] >= '0' && id[1] <= '9'
}

// Models lists models via GET /models.
func (p *Provider) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.Key == "" && !p.Compatible {
		return nil, apiutil.ErrNoKey
	}
	if p.BaseURL == "" {
		return nil, fmt.Errorf("%s: no base_url configured", p.name)
	}
	resp, err := apiutil.Do(ctx, p.name, "GET", apiutil.JoinURL(p.BaseURL, "/models"), p.headers(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"` // OpenRouter and some servers
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: decode models: %w", p.name, err)
	}
	var out []provider.ModelInfo
	for _, m := range v.Data {
		if !p.Compatible && !chatModel(m.ID) {
			continue
		}
		out = append(out, provider.ModelInfo{ID: m.ID, ContextWindow: m.ContextLength})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}
