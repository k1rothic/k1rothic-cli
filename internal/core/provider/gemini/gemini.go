// Package gemini adapts the Gemini API (streamGenerateContent, SSE) to the
// normalized provider interface. It is a hand-written HTTP client.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/apiutil"
	"github.com/akutor/k1rothic-cli/internal/core/provider/sse"
)

const (
	DefaultBaseURL = "https://generativelanguage.googleapis.com"
	DefaultModel   = "gemini-2.5-pro"
)

// Provider is the Gemini adapter.
type Provider struct {
	name    string
	BaseURL string
	Key     string
}

// NewCompatible returns an adapter for a Gemini-API endpoint at baseURL.
func NewCompatible(key, baseURL string) *Provider {
	return &Provider{name: "custom", BaseURL: baseURL, Key: key}
}

// New returns an adapter for key.
func New(key, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Provider{name: "gemini", BaseURL: baseURL, Key: key}
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) headers() map[string]string {
	return map[string]string{"x-goog-api-key": p.Key}
}

type part struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type thinkingConfig struct {
	IncludeThoughts bool   `json:"includeThoughts"`
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
}

type generationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type request struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

// thinkingFor maps effort to Gemini's controls: a token budget on the 2.x
// family, a thinking level on later models.
func thinkingFor(model string, e provider.Effort) *thinkingConfig {
	if e == "" {
		return nil
	}
	if strings.Contains(model, "gemini-2") {
		budgets := map[provider.Effort]int{provider.EffortOff: 0, provider.EffortLow: 1024, provider.EffortMedium: 8192, provider.EffortHigh: 24576}
		b := budgets[e]
		if b == 0 && strings.Contains(model, "pro") {
			b = 128 // Pro models cannot turn thinking off; use the minimum.
		}
		return &thinkingConfig{IncludeThoughts: b > 0, ThinkingBudget: &b}
	}
	level := "high"
	if e == provider.EffortOff || e == provider.EffortLow {
		level = "low"
	}
	return &thinkingConfig{IncludeThoughts: e != provider.EffortOff, ThinkingLevel: level}
}

func buildRequest(req provider.Request) request {
	var r request
	if req.System != "" {
		r.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}
	for _, m := range req.Messages {
		if m.Text == "" {
			continue
		}
		role := "user"
		if m.Role == provider.RoleAssistant {
			role = "model"
		}
		if n := len(r.Contents); n > 0 && r.Contents[n-1].Role == role {
			r.Contents[n-1].Parts = append(r.Contents[n-1].Parts, part{Text: m.Text})
			continue
		}
		r.Contents = append(r.Contents, content{Role: role, Parts: []part{{Text: m.Text}}})
	}
	gc := &generationConfig{MaxOutputTokens: req.MaxTokens, ThinkingConfig: thinkingFor(req.Model, req.Effort)}
	if gc.MaxOutputTokens > 0 || gc.ThinkingConfig != nil {
		r.GenerationConfig = gc
	}
	return r
}

type chunk struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error json.RawMessage `json:"error"`
}

// Stream implements provider.Provider.
func (p *Provider) Stream(ctx context.Context, req provider.Request) <-chan provider.Event {
	return apiutil.Run(ctx, func(out apiutil.Out) (provider.StopReason, error) {
		if p.Key == "" {
			return "", apiutil.ErrNoKey
		}
		u := apiutil.JoinURL(p.BaseURL, "/v1beta/models/"+url.PathEscape(req.Model)+":streamGenerateContent?alt=sse")
		resp, err := apiutil.Do(ctx, "gemini", "POST", u, p.headers(), buildRequest(req))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		return parseStream(resp.Body, out)
	})
}

func parseStream(body io.Reader, out apiutil.Out) (provider.StopReason, error) {
	rd := sse.NewReader(body)
	stop := provider.StopEndTurn
	sawData := false
	for {
		ev, err := rd.Next()
		if errors.Is(err, io.EOF) {
			if !sawData {
				return "", errors.New("gemini: empty response")
			}
			return stop, nil
		}
		if err != nil {
			return "", fmt.Errorf("gemini: %w", err)
		}
		if strings.TrimSpace(ev.Data) == "" {
			continue
		}
		sawData = true
		var c chunk
		if err := json.Unmarshal([]byte(ev.Data), &c); err != nil {
			continue
		}
		if len(c.Error) > 0 && string(c.Error) != "null" {
			return "", fmt.Errorf("gemini: %s", apiutil.ErrorMessage([]byte(`{"error":`+string(c.Error)+`}`)))
		}
		if c.PromptFeedback != nil && c.PromptFeedback.BlockReason != "" {
			return provider.StopRefusal, fmt.Errorf("gemini: prompt blocked (%s)", c.PromptFeedback.BlockReason)
		}
		for _, cand := range c.Candidates {
			for _, pt := range cand.Content.Parts {
				ok := true
				if pt.Thought {
					ok = out.Reasoning(pt.Text)
				} else {
					ok = out.Text(pt.Text)
				}
				if !ok {
					return "", nil
				}
			}
			switch cand.FinishReason {
			case "MAX_TOKENS":
				stop = provider.StopMaxTokens
			case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII":
				stop = provider.StopRefusal
			}
		}
		if m := c.UsageMetadata; m != nil {
			if !out.Usage(provider.Usage{
				InputTokens:     m.PromptTokenCount - m.CachedContentTokenCount,
				CacheReadTokens: m.CachedContentTokenCount,
				OutputTokens:    m.CandidatesTokenCount + m.ThoughtsTokenCount,
				ReasoningTokens: m.ThoughtsTokenCount,
			}) {
				return "", nil
			}
		}
	}
}

// Models lists models that support generateContent.
func (p *Provider) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.Key == "" {
		return nil, apiutil.ErrNoKey
	}
	resp, err := apiutil.Do(ctx, "gemini", "GET", apiutil.JoinURL(p.BaseURL, "/v1beta/models?pageSize=1000"), p.headers(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("gemini: decode models: %w", err)
	}
	var out []provider.ModelInfo
	for _, m := range v.Models {
		gen := false
		for _, s := range m.SupportedGenerationMethods {
			gen = gen || s == "generateContent"
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if !gen || !strings.HasPrefix(id, "gemini") {
			continue
		}
		out = append(out, provider.ModelInfo{ID: id, Name: m.DisplayName, ContextWindow: m.InputTokenLimit})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}
