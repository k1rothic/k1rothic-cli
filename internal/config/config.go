// Package config loads and saves the user's TOML configuration from
// os.UserConfigDir()/k1rothic/config.toml.
//
// Secrets never live here: API keys are stored only in the OS keychain.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/akutor/k1rothic-cli/internal/core/permissions"
)

// AppName is the directory name under the OS config dir.
const AppName = "k1rothic"

// Config is the persisted user configuration.
type Config struct {
	Provider string `toml:"provider" comment:"Provider: anthropic, openai, gemini, fake (offline demo), or custom:<name> for an [endpoints.<name>] entry"`
	Model    string `toml:"model"`
	Effort   string `toml:"effort" comment:"Reasoning effort: off, low, medium, high"`

	Approval approvalConfig `toml:"approval"`
	UI       UIConfig       `toml:"ui"`

	// Providers holds per-provider settings keyed by provider id. API keys
	// are never stored here.
	Providers map[string]ProviderConfig `toml:"providers,omitempty"`

	// Endpoints are the user's custom API servers, keyed by name; the
	// provider id of one is "custom:<name>".
	Endpoints map[string]ProviderConfig `toml:"endpoints,omitempty"`
}

// EndpointPrefix starts the provider id of a custom endpoint.
const EndpointPrefix = "custom:"

// EndpointName returns the endpoint name of a "custom:<name>" provider id.
func EndpointName(id string) (string, bool) {
	if strings.HasPrefix(id, EndpointPrefix) {
		return id[len(EndpointPrefix):], true
	}
	return "", false
}

// ValidEndpointName reports whether name is usable as an endpoint name:
// 1-32 letters, digits, '-' or '_'.
func ValidEndpointName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if !(r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// UpdateProvider edits the saved section for provider id (a built-in
// provider or a custom endpoint).
func (c *Config) UpdateProvider(id string, f func(pc *ProviderConfig)) {
	if name, ok := EndpointName(id); ok {
		if c.Endpoints == nil {
			c.Endpoints = map[string]ProviderConfig{}
		}
		pc := c.Endpoints[name]
		f(&pc)
		c.Endpoints[name] = pc
		return
	}
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}
	pc := c.Providers[id]
	f(&pc)
	c.Providers[id] = pc
}

// ProviderConfig is the per-provider section, e.g. [providers.custom].
type ProviderConfig struct {
	API     string `toml:"api,omitempty" comment:"API format: chat-completions, responses, anthropic-messages, gemini"`
	BaseURL string `toml:"base_url,omitempty" comment:"API base URL (required for custom, e.g. http://localhost:11434/v1)"`
	Model   string `toml:"model,omitempty" comment:"Last model used with this provider"`
}

// ProviderSettings returns the section for id (zero value if absent).
func (c Config) ProviderSettings(id string) ProviderConfig {
	if name, ok := EndpointName(id); ok {
		return c.Endpoints[name]
	}
	return c.Providers[id]
}

// SetProviderModel remembers model as the last one used with provider id.
func (c *Config) SetProviderModel(id, model string) {
	c.UpdateProvider(id, func(pc *ProviderConfig) { pc.Model = model })
}

type approvalConfig struct {
	Mode string `toml:"mode" comment:"ask, auto-edit, plan, always-approve"`
	// AllowAlwaysApprove is the explicit opt-in that makes always-approve
	// reachable. Without it, always-approve in Mode is rejected.
	AllowAlwaysApprove bool `toml:"allow_always_approve"`
}

// UIConfig holds presentation settings.
type UIConfig struct {
	Theme   string `toml:"theme" comment:"auto, dark, light"`
	Color   string `toml:"color" comment:"Colour theme: carbon, graphite, ocean, forest, amber, rose, violet, mono"`
	Spinner string `toml:"spinner" comment:"Loading animation: braille, dots, pulse, line (ASCII)"`
	Glyphs  string `toml:"glyphs" comment:"auto, unicode, safe (safe avoids symbols many fonts lack; auto = safe on Windows)"`
	Mouse   bool   `toml:"mouse" comment:"Capture the mouse wheel for scrolling (hold Shift to select text in most terminals)"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		Provider: "anthropic",
		Model:    "claude-opus-5-5",
		Effort:   "medium",
		Approval: approvalConfig{Mode: permissions.Ask.String()},
		UI:       UIConfig{Theme: "auto", Color: "carbon", Spinner: "braille", Glyphs: "auto", Mouse: true},
	}
}

// Dir returns the config directory, honouring K1ROTHIC_CONFIG_DIR for tests
// and portable installs.
func Dir() (string, error) {
	if d := os.Getenv("K1ROTHIC_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, AppName), nil
}

// Path returns the config file path.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// Load reads the config at path, falling back to defaults for a missing file
// or missing keys.
func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	// Tolerate CRLF and a UTF-8 BOM written by Windows editors.
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.migrate()
	return cfg, cfg.Validate()
}

// LegacyEndpoint is the endpoint name given to the single custom server
// of older versions ("openai-compatible", later "custom").
const LegacyEndpoint = "custom"

// migrate upgrades settings written by older versions: the single
// "openai-compatible"/"custom" provider becomes the endpoint "custom".
func (c *Config) migrate() {
	for _, old := range []string{"custom", "openai-compatible"} {
		if c.Provider == old {
			c.Provider = EndpointPrefix + LegacyEndpoint
		}
		pc, ok := c.Providers[old]
		if !ok {
			continue
		}
		if _, has := c.Endpoints[LegacyEndpoint]; !has {
			if pc.API == "" {
				pc.API = "chat-completions"
			}
			if c.Endpoints == nil {
				c.Endpoints = map[string]ProviderConfig{}
			}
			c.Endpoints[LegacyEndpoint] = pc
		}
		delete(c.Providers, old)
	}
	if len(c.Providers) == 0 {
		c.Providers = nil
	}
}

// Validate checks semantic constraints.
func (c Config) Validate() error {
	m, err := permissions.Parse(c.Approval.Mode)
	if err != nil {
		return err
	}
	if m == permissions.AlwaysApprove && !c.Approval.AllowAlwaysApprove {
		return errors.New("approval.mode = always-approve requires approval.allow_always_approve = true")
	}
	switch c.Effort {
	case "off", "low", "medium", "high":
	default:
		return fmt.Errorf("effort %q: want off, low, medium or high", c.Effort)
	}
	checkAPI := func(section, id, api string) error {
		switch api {
		case "", "chat-completions", "responses", "anthropic-messages", "gemini":
			return nil
		}
		return fmt.Errorf("%s.%s.api %q: want chat-completions, responses, anthropic-messages or gemini", section, id, api)
	}
	for id, pc := range c.Providers {
		if err := checkAPI("providers", id, pc.API); err != nil {
			return err
		}
	}
	for name, pc := range c.Endpoints {
		if !ValidEndpointName(name) {
			return fmt.Errorf("endpoints.%s: names may use letters, digits, '-' and '_' (max 32)", name)
		}
		if err := checkAPI("endpoints", name, pc.API); err != nil {
			return err
		}
	}
	if name, ok := EndpointName(c.Provider); ok {
		if _, exists := c.Endpoints[name]; !exists {
			return fmt.Errorf("provider %q: no [endpoints.%s] section", c.Provider, name)
		}
	}
	switch c.UI.Glyphs {
	case "auto", "unicode", "safe":
	default:
		return fmt.Errorf("ui.glyphs %q: want auto, unicode or safe", c.UI.Glyphs)
	}
	switch c.UI.Color {
	case "carbon", "graphite", "ocean", "forest", "amber", "rose", "violet", "mono":
	default:
		return fmt.Errorf("ui.color %q: want carbon, graphite, ocean, forest, amber, rose, violet or mono", c.UI.Color)
	}
	switch c.UI.Spinner {
	case "braille", "dots", "pulse", "line":
	default:
		return fmt.Errorf("ui.spinner %q: want braille, dots, pulse or line", c.UI.Spinner)
	}
	switch c.UI.Theme {
	case "auto", "dark", "light":
	default:
		return fmt.Errorf("ui.theme %q: want auto, dark or light", c.UI.Theme)
	}
	return nil
}

// ApprovalMode returns the parsed approval mode (Ask on error).
func (c Config) ApprovalMode() permissions.Mode {
	m, err := permissions.Parse(c.Approval.Mode)
	if err != nil || (m == permissions.AlwaysApprove && !c.Approval.AllowAlwaysApprove) {
		return permissions.Ask
	}
	return m
}

// AllowAlwaysApprove reports the explicit opt-in.
func (c Config) AllowAlwaysApprove() bool { return c.Approval.AllowAlwaysApprove }

// Save writes cfg atomically (temp file + rename) with user-only permissions.
func Save(path string, cfg Config) error {
	b, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
