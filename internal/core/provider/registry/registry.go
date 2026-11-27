// Package registry lists the Mode A providers, resolves their API keys from
// the keychain and constructs adapters.
package registry

import (
	"fmt"
	"strings"

	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/anthropic"
	"github.com/akutor/k1rothic-cli/internal/core/provider/fake"
	"github.com/akutor/k1rothic-cli/internal/core/provider/gemini"
	"github.com/akutor/k1rothic-cli/internal/core/provider/openai"
	"github.com/akutor/k1rothic-cli/internal/secrets"
)

// Info describes a provider.
type Info struct {
	ID           string
	Label        string
	DefaultModel string
	KeyRequired  bool   // false: a key is optional (local servers) or unused
	KeyURL       string // where the user creates a key
	NeedsBaseURL bool
}

// All providers in display order.
var All = []Info{
	{ID: "anthropic", Label: "Anthropic", DefaultModel: anthropic.DefaultModel, KeyRequired: true, KeyURL: "https://platform.claude.com/settings/keys"},
	{ID: "openai", Label: "OpenAI", DefaultModel: openai.DefaultModel, KeyRequired: true, KeyURL: "https://platform.openai.com/api-keys"},
	{ID: "gemini", Label: "Google Gemini", DefaultModel: gemini.DefaultModel, KeyRequired: true, KeyURL: "https://aistudio.google.com/apikey"},
	{ID: "fake", Label: "Demo (offline, scripted)", DefaultModel: "fake-1"},
}

// API formats a provider can speak.
const (
	APIChat      = "chat-completions"
	APIResponses = "responses"
	APIMessages  = "anthropic-messages"
	APIGemini    = "gemini"
)

// APIInfo describes an API format.
type APIInfo struct {
	ID      string
	Label   string
	Example string // typical base URL for a custom endpoint
	Hint    string
}

// APIs lists every format in display order.
var APIs = []APIInfo{
	{ID: APIChat, Label: "OpenAI Chat Completions", Example: "http://localhost:11434/v1",
		Hint: "Ollama, vLLM, LM Studio, llama.cpp, OpenRouter, Groq, Together, DeepSeek…"},
	{ID: APIResponses, Label: "OpenAI Responses", Example: "https://api.openai.com/v1",
		Hint: "OpenAI's newer API with reasoning summaries; also vLLM, OpenRouter, Azure"},
	{ID: APIMessages, Label: "Anthropic Messages", Example: "https://api.deepseek.com/anthropic",
		Hint: "Claude-compatible gateways: DeepSeek, Kimi, MiniMax, GLM, OpenRouter, LiteLLM…"},
	{ID: APIGemini, Label: "Google Gemini", Example: "https://generativelanguage.googleapis.com",
		Hint: "Gemini API or a proxy in front of it"},
}

// LookupAPI returns the format with id.
func LookupAPI(id string) (APIInfo, bool) {
	for _, a := range APIs {
		if a.ID == id {
			return a, true
		}
	}
	return APIInfo{}, false
}

// EndpointPrefix starts the provider id of a custom endpoint
// ("custom:<name>"); it matches config.EndpointPrefix.
const EndpointPrefix = "custom:"

// IsEndpoint reports whether id names a custom endpoint.
func IsEndpoint(id string) bool { return strings.HasPrefix(id, EndpointPrefix) }

// APIsFor lists the formats provider id can use; the first is the default.
func APIsFor(id string) []string {
	if IsEndpoint(id) {
		id = "custom"
	}
	switch id {
	case "anthropic":
		return []string{APIMessages}
	case "openai":
		return []string{APIChat, APIResponses}
	case "gemini":
		return []string{APIGemini}
	case "custom":
		return []string{APIChat, APIResponses, APIMessages, APIGemini}
	}
	return nil
}

// APIFor resolves the configured format for id (default if unset/invalid).
func APIFor(id, configured string) string {
	opts := APIsFor(id)
	for _, o := range opts {
		if o == configured {
			return o
		}
	}
	if len(opts) > 0 {
		return opts[0]
	}
	return ""
}

// legacyKeyIDs are where the key of the single custom server of older
// versions was stored. They are only read (for the endpoint
// "custom:custom"), never written.
var legacyKeyIDs = []string{"custom", "openai-compatible"}

// Lookup returns the provider with id. Custom endpoints ("custom:<name>")
// are described generically.
func Lookup(id string) (Info, bool) {
	if IsEndpoint(id) {
		name := id[len(EndpointPrefix):]
		return Info{ID: id, Label: name, NeedsBaseURL: true}, name != ""
	}
	for _, i := range All {
		if i.ID == id {
			return i, true
		}
	}
	return Info{}, false
}

// Registry builds adapters using keys from Store.
type Registry struct {
	Store secrets.Store
}

// New returns a registry over store.
func New(store secrets.Store) *Registry { return &Registry{Store: store} }

// KeyStatus reports where id's key comes from (secrets.None if missing).
func (r *Registry) KeyStatus(id string) (secrets.Source, error) {
	if id == "fake" {
		return secrets.None, nil
	}
	_, src, err := r.key(id)
	return src, err
}

func (r *Registry) key(id string) (string, secrets.Source, error) {
	k, src, err := r.Store.Get(id)
	if id == EndpointPrefix+"custom" && src == secrets.None && err == nil {
		for _, old := range legacyKeyIDs {
			if k, src, err := r.Store.Get(old); err != nil || src != secrets.None {
				return k, src, err
			}
		}
	}
	return k, src, err
}

// Open constructs the adapter for id. A missing key is not an error here:
// the adapter reports it on the first request so the UI can still start.
func (r *Registry) Open(id, baseURL, api string) (provider.Provider, error) {
	var key string
	if id != "fake" {
		k, _, err := r.key(id)
		if err != nil {
			return nil, err
		}
		key = k
	}
	if IsEndpoint(id) {
		switch APIFor(id, api) {
		case APIResponses:
			return openai.NewCompatible(key, baseURL).WithResponses(), nil
		case APIMessages:
			return anthropic.NewCompatible(key, baseURL), nil
		case APIGemini:
			return gemini.NewCompatible(key, baseURL), nil
		}
		return openai.NewCompatible(key, baseURL), nil
	}
	switch id {
	case "anthropic":
		return anthropic.New(key, baseURL), nil
	case "openai":
		if APIFor(id, api) == APIResponses {
			return openai.New(key, baseURL).WithResponses(), nil
		}
		return openai.New(key, baseURL), nil
	case "gemini":
		return gemini.New(key, baseURL), nil
	case "fake":
		return fake.New(), nil
	}
	return nil, fmt.Errorf("unknown provider %q", id)
}

// ContextWindow is a static fallback for models whose listing does not
// report a context size. 0 means unknown.
func ContextWindow(model string) int {
	m := strings.ToLower(model)
	switch {
	case m == "fake-1":
		return 500_000
	case strings.HasPrefix(m, "claude-haiku-4"), strings.HasPrefix(m, "claude-3"):
		return 200_000
	case strings.HasPrefix(m, "claude-"):
		return 1_000_000
	case strings.HasPrefix(m, "gpt-5"):
		return 400_000
	case strings.HasPrefix(m, "gpt-4.1"):
		return 1_047_576
	case strings.HasPrefix(m, "gpt-4o"):
		return 128_000
	case len(m) > 1 && m[0] == 'o' && m[1] >= '0' && m[1] <= '9':
		return 200_000
	case strings.HasPrefix(m, "gemini-"):
		return 1_048_576
	}
	return 0
}
