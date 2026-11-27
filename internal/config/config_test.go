package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/akutor/k1rothic-cli/internal/core/permissions"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("got %+v", cfg)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	in := Default()
	in.Model = "m"
	in.UI.Theme = "light"
	in.Providers = map[string]ProviderConfig{"openai": {API: "responses", Model: "gpt-x"}}
	in.Endpoints = map[string]ProviderConfig{"gw": {API: "anthropic-messages", BaseURL: "https://gw.example/anthropic", Model: "m1"}}
	in.Provider = "custom:gw"
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round trip: %+v != %+v", out, in)
	}
}

func TestLoadCRLFAndBOM(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	data := "\xEF\xBB\xBFmodel = \"x\"\r\n[ui]\r\ntheme = \"dark\"\r\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "x" || cfg.UI.Theme != "dark" || cfg.Effort != "medium" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestAlwaysApproveRequiresOptIn(t *testing.T) {
	c := Default()
	c.Approval.Mode = "always-approve"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error without opt-in")
	}
	if c.ApprovalMode() != permissions.Ask {
		t.Fatal("must fall back to ask")
	}
	c.Approval.AllowAlwaysApprove = true
	if err := c.Validate(); err != nil || c.ApprovalMode() != permissions.AlwaysApprove {
		t.Fatalf("opt-in not honoured: %v", err)
	}
}

func TestMigratesOpenAICompatibleToCustom(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	data := `provider = "openai-compatible"
[providers.openai-compatible]
base_url = "http://localhost:11434/v1"
model = "llama"
`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	pc := cfg.Endpoints["custom"]
	if cfg.Provider != "custom:custom" || pc.API != "chat-completions" || pc.BaseURL != "http://localhost:11434/v1" || pc.Model != "llama" {
		t.Fatalf("not migrated: %+v", cfg)
	}
	if _, ok := cfg.Providers["openai-compatible"]; ok {
		t.Fatal("old section kept")
	}
}

func TestRejectsUnknownAPI(t *testing.T) {
	c := Default()
	c.Endpoints = map[string]ProviderConfig{"x": {API: "soap"}}
	if c.Validate() == nil {
		t.Fatal("want error")
	}
}

func TestEndpointValidation(t *testing.T) {
	c := Default()
	c.Provider = "custom:missing"
	if c.Validate() == nil {
		t.Fatal("provider pointing at a missing endpoint must fail")
	}
	c = Default()
	c.Endpoints = map[string]ProviderConfig{"bad name": {}}
	if c.Validate() == nil {
		t.Fatal("invalid endpoint name must fail")
	}
}

func TestMigratesSingleCustomProvider(t *testing.T) {
	c := Default()
	c.Provider = "custom"
	c.Providers = map[string]ProviderConfig{"custom": {API: "responses", BaseURL: "http://x"}}
	c.migrate()
	if c.Provider != "custom:custom" || c.Endpoints["custom"].API != "responses" || c.Providers != nil {
		t.Fatalf("%+v", c)
	}
}
