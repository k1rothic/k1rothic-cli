package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/anthropic"
	"github.com/akutor/k1rothic-cli/internal/core/provider/openai"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
	"github.com/akutor/k1rothic-cli/internal/secrets"
)

const responsesSSE = `event: response.created
data: {"type":"response.created","response":{"status":"in_progress"}}

event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","delta":"weighing"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hi"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"!"}

event: response.completed
data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":50,"output_tokens":9,"input_tokens_details":{"cached_tokens":20},"output_tokens_details":{"reasoning_tokens":4}}}}

`

func TestResponsesStream(t *testing.T) {
	s, c := server(t, 200, responsesSSE)
	r := collect(t, openai.New("sk", s.URL).WithResponses().Stream(context.Background(), req))
	if r.err != nil || r.text != "Hi!" || r.reasoning != "weighing" || r.stop != provider.StopEndTurn {
		t.Fatalf("%+v", r)
	}
	if r.usage != (provider.Usage{InputTokens: 30, CacheReadTokens: 20, OutputTokens: 9, ReasoningTokens: 4}) {
		t.Fatalf("usage %+v", r.usage)
	}
	if c.path != "/responses" || c.body["instructions"] != "be brief" || c.body["store"] != false {
		t.Fatalf("request %s %v", c.path, c.body)
	}
	rs := c.body["reasoning"].(map[string]any)
	if rs["effort"] != "high" || rs["summary"] != "auto" {
		t.Fatalf("reasoning %v", rs)
	}
	in := c.body["input"].([]any)
	if len(in) != 3 || in[1].(map[string]any)["role"] != "assistant" {
		t.Fatalf("input %v", in)
	}
}

func TestResponsesFailed(t *testing.T) {
	s, _ := server(t, 200, "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"model overloaded\"}}}\n\n")
	r := collect(t, openai.NewCompatible("", s.URL).WithResponses().Stream(context.Background(), req))
	if r.err == nil || !strings.Contains(r.err.Error(), "model overloaded") {
		t.Fatalf("err = %v", r.err)
	}
}

func TestAnthropicCompatibleGateway(t *testing.T) {
	s, c := server(t, 200, anthropicSSE)
	r := collect(t, anthropic.NewCompatible("gw-key", s.URL+"/anthropic").Stream(context.Background(), req))
	if r.err != nil || r.text != "Hello" {
		t.Fatalf("%+v", r)
	}
	if c.path != "/anthropic/v1/messages" {
		t.Fatalf("path %s", c.path)
	}
	if c.headers.Get("anthropic-version") != anthropic.CompatAPIVersion ||
		c.headers.Get("x-api-key") != "gw-key" || c.headers.Get("Authorization") != "Bearer gw-key" {
		t.Fatalf("headers %v", c.headers)
	}
	th := c.body["thinking"].(map[string]any)
	if th["type"] != "enabled" || th["budget_tokens"].(float64) != 16000 {
		t.Fatalf("gateways get a thinking budget, got %v", th)
	}
	if _, ok := c.body["output_config"]; ok {
		t.Fatal("gateways must not get output_config")
	}
}

func TestRegistryBuildsEachAPIFormat(t *testing.T) {
	r := registry.New(secrets.Mem{"custom:gw": "k", "openai": "k"})
	cases := []struct {
		id, api string
		check   func(provider.Provider) bool
	}{
		{"custom:gw", "", func(p provider.Provider) bool { o, ok := p.(*openai.Provider); return ok && !o.Responses }},
		{"custom:gw", registry.APIResponses, func(p provider.Provider) bool { o, ok := p.(*openai.Provider); return ok && o.Responses }},
		{"custom:gw", registry.APIMessages, func(p provider.Provider) bool { a, ok := p.(*anthropic.Provider); return ok && a.Compatible }},
		{"custom:gw", registry.APIGemini, func(p provider.Provider) bool { return p.Name() == "custom" && p != nil }},
		{"openai", registry.APIResponses, func(p provider.Provider) bool { o, ok := p.(*openai.Provider); return ok && o.Responses }},
		{"openai", registry.APIMessages, func(p provider.Provider) bool { o, ok := p.(*openai.Provider); return ok && !o.Responses }}, // invalid → default
	}
	for _, tc := range cases {
		p, err := r.Open(tc.id, "http://x", tc.api)
		if err != nil || !tc.check(p) {
			t.Errorf("%s/%s: got %T %v", tc.id, tc.api, p, err)
		}
	}
}

func TestCustomFallsBackToLegacyKey(t *testing.T) {
	r := registry.New(secrets.Mem{"openai-compatible": "old"})
	src, err := r.KeyStatus("custom:custom")
	if err != nil || src != secrets.Keychain {
		t.Fatalf("src=%v err=%v", src, err)
	}
}
