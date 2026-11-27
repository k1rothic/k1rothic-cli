// Package anthropic adapts the Claude Messages API (streaming SSE) to the
// normalized provider interface. It is a hand-written HTTP client.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/apiutil"
	"github.com/akutor/k1rothic-cli/internal/core/provider/sse"
)

const (
	DefaultBaseURL = "https://api.anthropic.com"
	APIVersion     = "2026-10-21"
	DefaultModel   = "claude-opus-5-5"
	defaultMaxTok  = 32000
)

// Provider is the Anthropic adapter.
type Provider struct {
	name    string
	BaseURL string
	Key     string
	// Compatible targets third-party gateways that speak the Messages API
	// (DeepSeek, Kimi, MiniMax, OpenRouter, LiteLLM, …): the widely
	// supported 2023-06-01 version, Bearer auth alongside x-api-key, and a
	// thinking token budget instead of adaptive thinking.
	Compatible bool
}

// NewCompatible returns an adapter for a Messages-API gateway at baseURL
// (the part before /v1/messages).
func NewCompatible(key, baseURL string) *Provider {
	return &Provider{name: "custom", BaseURL: baseURL, Key: key, Compatible: true}
}

// New returns an adapter for key.
func New(key, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Provider{name: "anthropic", BaseURL: baseURL, Key: key}
}

func (p *Provider) Name() string { return p.name }

// CompatAPIVersion is the version third-party gateways understand.
const CompatAPIVersion = "2023-06-01"

func (p *Provider) headers() map[string]string {
	if p.Compatible {
		h := map[string]string{"anthropic-version": CompatAPIVersion}
		if p.Key != "" {
			h["x-api-key"] = p.Key
			h["Authorization"] = "Bearer " + p.Key
		}
		return h
	}
	return map[string]string{"x-api-key": p.Key, "anthropic-version": APIVersion}
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type message struct {
	Role    string      `json:"role"`
	Content []textBlock `json:"content"`
}

type thinking struct {
	Type         string `json:"type"`
	Display      string `json:"display,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// compatBudgets maps effort to a thinking budget for gateways.
var compatBudgets = map[provider.Effort]int{provider.EffortLow: 2048, provider.EffortMedium: 8192, provider.EffortHigh: 16000}

type outputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type request struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system,omitempty"`
	Messages     []message     `json:"messages"`
	Stream       bool          `json:"stream"`
	Thinking     *thinking     `json:"thinking,omitempty"`
	OutputConfig *outputConfig `json:"output_config,omitempty"`
}

// buildRequest maps the normalized request. Effort "off" asks for the
// lowest effort without visible reasoning: current Claude models keep
// thinking on, so it cannot be fully disabled.
func buildRequest(req provider.Request) request { return buildRequestFor(req, false) }

func buildRequestFor(req provider.Request, compatible bool) request {
	r := request{Model: req.Model, MaxTokens: req.MaxTokens, System: req.System, Stream: true}
	if r.MaxTokens <= 0 {
		r.MaxTokens = defaultMaxTok
	}
	for _, m := range req.Messages {
		role := "user"
		if m.Role == provider.RoleAssistant {
			role = "assistant"
		}
		if m.Text == "" {
			continue
		}
		// Merge consecutive same-role turns (the API requires alternation).
		if n := len(r.Messages); n > 0 && r.Messages[n-1].Role == role {
			r.Messages[n-1].Content = append(r.Messages[n-1].Content, textBlock{Type: "text", Text: m.Text})
			continue
		}
		r.Messages = append(r.Messages, message{Role: role, Content: []textBlock{{Type: "text", Text: m.Text}}})
	}
	if compatible {
		if b, ok := compatBudgets[req.Effort]; ok {
			r.Thinking = &thinking{Type: "enabled", BudgetTokens: b}
		}
		return r
	}
	switch req.Effort {
	case provider.EffortOff:
		r.OutputConfig = &outputConfig{Effort: "low"}
	case provider.EffortLow, provider.EffortMedium, provider.EffortHigh:
		r.Thinking = &thinking{Type: "adaptive", Display: "summarized"}
		r.OutputConfig = &outputConfig{Effort: string(req.Effort)}
	}
	return r
}

type usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Usage usage `json:"usage"`
	} `json:"message"`
	Delta struct {
		Type       string `json:"type"`
		Text       string `json:"text"`
		Thinking   string `json:"thinking"`
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage *usage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Stream implements provider.Provider.
func (p *Provider) Stream(ctx context.Context, req provider.Request) <-chan provider.Event {
	return apiutil.Run(ctx, func(out apiutil.Out) (provider.StopReason, error) {
		if p.Key == "" && !p.Compatible {
			return "", apiutil.ErrNoKey
		}
		if p.BaseURL == "" {
			return "", fmt.Errorf("%s: no base URL configured — run /provider", p.name)
		}
		resp, err := apiutil.Do(ctx, p.name, "POST", apiutil.JoinURL(p.BaseURL, "/v1/messages"), p.headers(), buildRequestFor(req, p.Compatible))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		return parseStream(resp.Body, out)
	})
}

func parseStream(body io.Reader, out apiutil.Out) (provider.StopReason, error) {
	rd := sse.NewReader(body)
	var u provider.Usage
	stop := provider.StopEndTurn
	sawStop := false
	for {
		ev, err := rd.Next()
		if errors.Is(err, io.EOF) {
			if !sawStop {
				return "", errors.New("anthropic: stream ended unexpectedly")
			}
			return stop, nil
		}
		if err != nil {
			return "", fmt.Errorf("anthropic: %w", err)
		}
		if ev.Data == "" {
			continue
		}
		var se streamEvent
		if err := json.Unmarshal([]byte(ev.Data), &se); err != nil {
			continue // unknown/forward-compatible payload
		}
		switch se.Type {
		case "message_start":
			mu := se.Message.Usage
			u = provider.Usage{InputTokens: mu.InputTokens, OutputTokens: mu.OutputTokens,
				CacheReadTokens: mu.CacheReadInputTokens, CacheWriteTokens: mu.CacheCreationInputTokens}
			if !out.Usage(u) {
				return "", nil
			}
		case "content_block_delta":
			ok := true
			switch se.Delta.Type {
			case "text_delta":
				ok = out.Text(se.Delta.Text)
			case "thinking_delta":
				ok = out.Reasoning(se.Delta.Thinking)
			}
			if !ok {
				return "", nil
			}
		case "message_delta":
			if se.Usage != nil {
				u.OutputTokens = se.Usage.OutputTokens
				if se.Usage.InputTokens > 0 {
					u.InputTokens = se.Usage.InputTokens
				}
				if !out.Usage(u) {
					return "", nil
				}
			}
			switch se.Delta.StopReason {
			case "max_tokens":
				stop = provider.StopMaxTokens
			case "tool_use":
				stop = provider.StopToolUse
			case "refusal":
				stop = provider.StopRefusal
			}
		case "message_stop":
			sawStop = true
			if stop == provider.StopRefusal {
				return stop, errors.New("anthropic: the model declined this request (refusal)")
			}
			return stop, nil
		case "error":
			msg := "stream error"
			if se.Error != nil {
				msg = se.Error.Message
			}
			return "", fmt.Errorf("anthropic: %s", msg)
		}
	}
}

// Models lists available models via GET /v1/models.
func (p *Provider) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.Key == "" && !p.Compatible {
		return nil, apiutil.ErrNoKey
	}
	resp, err := apiutil.Do(ctx, p.name, "GET", apiutil.JoinURL(p.BaseURL, "/v1/models?limit=1000"), p.headers(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		Data []struct {
			ID             string `json:"id"`
			DisplayName    string `json:"display_name"`
			MaxInputTokens int    `json:"max_input_tokens"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("anthropic: decode models: %w", err)
	}
	out := make([]provider.ModelInfo, 0, len(v.Data))
	for _, m := range v.Data {
		out = append(out, provider.ModelInfo{ID: m.ID, Name: m.DisplayName, ContextWindow: m.MaxInputTokens})
	}
	return out, nil
}
