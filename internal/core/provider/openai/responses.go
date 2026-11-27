package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/apiutil"
	"github.com/akutor/k1rothic-cli/internal/core/provider/sse"
)

// Responses API (POST /responses). Used for OpenAI when selected and for
// custom endpoints that speak it (vLLM, Ollama, OpenRouter, …).

type respInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type respRequest struct {
	Model           string         `json:"model"`
	Instructions    string         `json:"instructions,omitempty"`
	Input           []respInput    `json:"input"`
	Stream          bool           `json:"stream"`
	Store           bool           `json:"store"`
	Reasoning       *respReasoning `json:"reasoning,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
}

func buildResponsesRequest(req provider.Request) respRequest {
	r := respRequest{Model: req.Model, Instructions: req.System, Stream: true, MaxOutputTokens: req.MaxTokens}
	for _, m := range req.Messages {
		if m.Text == "" {
			continue
		}
		role := "user"
		if m.Role == provider.RoleAssistant {
			role = "assistant"
		}
		r.Input = append(r.Input, respInput{Role: role, Content: m.Text})
	}
	if req.Effort != "" && req.Effort != provider.EffortOff {
		r.Reasoning = &respReasoning{Effort: string(req.Effort), Summary: "auto"}
	}
	return r
}

type respUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type respEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Message  string `json:"message"`
	Response *struct {
		Status            string     `json:"status"`
		Usage             *respUsage `json:"usage"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
}

func parseResponsesStream(name string, body io.Reader, out apiutil.Out) (provider.StopReason, error) {
	rd := sse.NewReader(body)
	for {
		ev, err := rd.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%s: stream ended before the response completed", name)
		}
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		data := strings.TrimSpace(ev.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var e respEvent
		if json.Unmarshal([]byte(data), &e) != nil {
			continue
		}
		switch e.Type {
		case "response.output_text.delta":
			if !out.Text(e.Delta) {
				return "", nil
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if !out.Reasoning(e.Delta) {
				return "", nil
			}
		case "response.reasoning_summary_part.done":
			if !out.Reasoning("\n\n") {
				return "", nil
			}
		case "response.completed", "response.incomplete":
			stop := provider.StopEndTurn
			if r := e.Response; r != nil {
				if u := r.Usage; u != nil {
					cached := u.InputTokensDetails.CachedTokens
					out.Usage(provider.Usage{
						InputTokens:     u.InputTokens - cached,
						CacheReadTokens: cached,
						OutputTokens:    u.OutputTokens,
						ReasoningTokens: u.OutputTokensDetails.ReasoningTokens,
					})
				}
				if d := r.IncompleteDetails; d != nil {
					switch d.Reason {
					case "max_output_tokens":
						stop = provider.StopMaxTokens
					case "content_filter":
						stop = provider.StopRefusal
					}
				}
			}
			return stop, nil
		case "response.failed":
			msg := "response failed"
			if e.Response != nil && e.Response.Error != nil {
				msg = e.Response.Error.Message
			}
			return "", fmt.Errorf("%s: %s", name, msg)
		case "error":
			msg := e.Message
			if msg == "" {
				msg = apiutil.ErrorMessage([]byte(data))
			}
			return "", fmt.Errorf("%s: %s", name, msg)
		}
	}
}
