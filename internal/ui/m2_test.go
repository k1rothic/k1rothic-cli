package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/permissions"
	"github.com/akutor/k1rothic-cli/internal/core/provider/anthropic"
	"github.com/akutor/k1rothic-cli/internal/core/provider/openai"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
	"github.com/akutor/k1rothic-cli/internal/secrets"
)

func newRegistryModel(t *testing.T, store secrets.Mem) (*Model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	m := New(Options{Config: testConfig(), ConfigPath: path, Registry: registry.New(store), Cwd: t.TempDir()})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, path
}

func viewLines(m *Model) []string { return strings.Split(plainView(m), "\n") }

func TestSingleRealCursorInInput(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	if m.input.VirtualCursor() {
		t.Fatal("textarea must not draw its own cursor")
	}
	typeText(m, "sss")
	v := m.View()
	if v.Cursor == nil {
		t.Fatal("expected the terminal cursor to be placed")
	}
	lines := strings.Split(ansi.Strip(v.Content), "\n")
	line := lines[v.Cursor.Y]
	if !strings.Contains(line, m.g.Prompt+" sss") {
		t.Fatalf("cursor row %d is %q", v.Cursor.Y, line)
	}
	want := ansi.StringWidth(line[:strings.Index(line, "sss")]) + 3
	if v.Cursor.X != want {
		t.Fatalf("cursor x = %d, want %d (after the text)", v.Cursor.X, want)
	}
}

func TestOuterVerticalPadding(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	lines := viewLines(m)
	if strings.TrimSpace(lines[0]) != "" || strings.TrimSpace(lines[len(lines)-1]) != "" {
		t.Fatalf("missing blank rows at the top/bottom of the screen:\n%s", plainView(m))
	}
	if !strings.Contains(lines[1], "500K") || !strings.Contains(lines[len(lines)-2], "Shift+Tab") {
		t.Fatalf("top bar / footer not inset by one row:\n%s", plainView(m))
	}
	// The input box itself has no inner blank rows.
	for i, l := range lines {
		if strings.Contains(l, "Ask anything") && !strings.Contains(lines[i-1], m.g.TL) {
			t.Fatalf("input box has inner padding:\n%s", plainView(m))
		}
	}
}

func TestShortConversationSitsAboveInput(t *testing.T) {
	m := newTestModel(t, 100, 40, nil)
	typeText(m, "hi")
	press(m, "enter")
	drain(t, m)
	lines := viewLines(m)
	box := -1
	for i, l := range lines {
		if strings.Contains(l, m.g.TL) {
			box = i
			break
		}
	}
	// box-1 is a spacer, box-2 the (empty) status row; the answer is right above.
	if box < 3 || !strings.Contains(lines[box-3], "Hello world") {
		t.Fatalf("gap between content and input:\n%s", plainView(m))
	}
}

func TestSettingsChangeAndPersist(t *testing.T) {
	m, path := newRegistryModel(t, secrets.Mem{})
	m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	if _, ok := m.panel.(*settingsPanel); !ok {
		t.Fatal("F2 should open settings")
	}
	if !strings.Contains(plainView(m), "Settings") || !strings.Contains(plainView(m), "Reasoning effort") {
		t.Fatalf("settings not shown:\n%s", plainView(m))
	}
	if m.View().Cursor != nil {
		t.Fatal("no text cursor while settings are open")
	}
	for range 4 { // Provider, Model, Custom endpoints, API format → Reasoning effort
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.cfg.Effort != "medium" {
		t.Fatalf("effort = %q", m.cfg.Effort)
	}
	press(m, "esc")
	if m.panel != nil {
		t.Fatal("esc should close settings")
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort != "medium" {
		t.Fatalf("saved effort = %q", got.Effort)
	}
}

func TestAllowAlwaysApproveNeedsConfirmation(t *testing.T) {
	m, _ := newRegistryModel(t, secrets.Mem{})
	typeText(m, "/settings")
	press(m, "enter")
	for range 6 { // → Allow always-approve
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	press(m, "enter")
	if m.allowAlways {
		t.Fatal("first press must only ask for confirmation")
	}
	if !strings.Contains(plainView(m), "press again to confirm") {
		t.Fatalf("no confirmation hint:\n%s", plainView(m))
	}
	press(m, "enter")
	if !m.allowAlways || !m.cfg.Approval.AllowAlwaysApprove {
		t.Fatal("second press should opt in")
	}
	// Approval mode cycle now reaches always-approve, with the footer warning.
	press(m, "esc")
	for i := 0; i < 5 && m.approval != permissions.AlwaysApprove; i++ {
		press(m, "shift+tab")
	}
	if m.approval != permissions.AlwaysApprove {
		t.Fatal("always-approve not reachable after opt-in")
	}
	if !strings.Contains(plainView(m), "always-approve") {
		t.Fatal("missing persistent warning")
	}
}

func TestProviderSwitchAsksForKeyAndStoresIt(t *testing.T) {
	store := secrets.Mem{}
	m, path := newRegistryModel(t, store)
	typeText(m, "/provider anthropic")
	press(m, "enter")
	if _, ok := m.panel.(*prompt); !ok {
		t.Fatalf("expected key prompt, panel = %T", m.panel)
	}
	m.Update(tea.PasteMsg{Content: "sk-test-123\n"})
	if strings.Contains(plainView(m), "sk-test-123") {
		t.Fatal("key must be masked on screen")
	}
	press(m, "enter")
	if store["anthropic"] != "sk-test-123" {
		t.Fatalf("store = %v", store)
	}
	if m.cfg.Provider != "anthropic" || m.prov.Name() != "anthropic" || m.cfg.Model == "" {
		t.Fatalf("provider not switched: %s %v %s", m.cfg.Provider, m.prov.Name(), m.cfg.Model)
	}
	got, _ := config.Load(path)
	if got.Provider != "anthropic" {
		t.Fatalf("saved provider = %q", got.Provider)
	}
	b, _ := readFile(path)
	if strings.Contains(b, "sk-test") {
		t.Fatal("API key leaked into config file")
	}
}

func TestModelCommandRemembersPerProvider(t *testing.T) {
	m, _ := newRegistryModel(t, secrets.Mem{})
	typeText(m, "/model fake-2")
	press(m, "enter")
	if m.cfg.Model != "fake-2" || m.cfg.ProviderSettings("fake").Model != "fake-2" {
		t.Fatalf("model not set: %+v", m.cfg)
	}
	if !strings.Contains(plainView(m), "fake-2 (low)") {
		t.Fatal("label not updated")
	}
}

func TestModelPickerAcceptsCustomID(t *testing.T) {
	m, _ := newRegistryModel(t, secrets.Mem{})
	typeText(m, "/model")
	press(m, "enter")
	if _, ok := m.panel.(*picker); !ok {
		t.Fatalf("panel = %T", m.panel)
	}
	typeText(m, "my-model")
	press(m, "enter")
	if m.cfg.Model != "my-model" {
		t.Fatalf("model = %q", m.cfg.Model)
	}
}

func TestMissingKeyNoticeOnStart(t *testing.T) {
	cfg := testConfig()
	cfg.Provider, cfg.Model = "anthropic", "claude-opus-5-5"
	m := New(Options{Config: cfg, Registry: registry.New(secrets.Mem{}), Cwd: t.TempDir()})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(plainView(m), "No Anthropic API key") {
		t.Fatalf("missing startup hint:\n%s", plainView(m))
	}
	if m.ctxLimit != 1_000_000 {
		t.Fatalf("ctxLimit = %d", m.ctxLimit)
	}
}

func readFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}

func TestCustomEndpointSetupWithPaste(t *testing.T) {
	store := secrets.Mem{}
	m, path := newRegistryModel(t, store)
	typeText(m, "/provider custom")
	press(m, "enter")
	if _, ok := m.panel.(*prompt); !ok || !strings.Contains(plainView(m), "Name:") {
		t.Fatalf("expected endpoint name prompt:\n%s", plainView(m))
	}
	typeText(m, "gw")
	press(m, "enter")
	p, ok := m.panel.(*picker)
	if !ok || !strings.Contains(plainView(m), "Anthropic Messages") || !strings.Contains(plainView(m), "OpenAI Responses") {
		t.Fatalf("expected API format picker:\n%s", plainView(m))
	}
	typeText(m, "anthropic") // filter
	press(m, "enter")
	_ = p
	if _, ok := m.panel.(*prompt); !ok {
		t.Fatalf("expected base URL prompt, panel = %T", m.panel)
	}
	m.Update(tea.PasteMsg{Content: "https://gw.example/anthropic/\r\n"})
	if !strings.Contains(plainView(m), "https://gw.example/anthropic") {
		t.Fatalf("pasted URL not in field:\n%s", plainView(m))
	}
	press(m, "enter")
	if _, ok := m.panel.(*prompt); !ok {
		t.Fatalf("expected key prompt, panel = %T", m.panel)
	}
	m.Update(tea.PasteMsg{Content: "gw-key"})
	press(m, "enter")
	if store["custom:gw"] != "gw-key" || m.cfg.Provider != "custom:gw" {
		t.Fatalf("store=%v provider=%s", store, m.cfg.Provider)
	}
	if _, ok := m.prov.(*anthropic.Provider); !ok {
		t.Fatalf("adapter = %T, want Anthropic Messages", m.prov)
	}
	got, _ := config.Load(path)
	if pc := got.Endpoints["gw"]; pc.API != "anthropic-messages" || pc.BaseURL != "https://gw.example/anthropic" {
		t.Fatalf("saved %+v", pc)
	}
	if _, ok := m.panel.(*picker); !ok {
		t.Fatalf("a custom endpoint has no default model; expected the model picker, panel = %T", m.panel)
	}
	press(m, "esc")
	// Switch the same endpoint to Chat Completions via /provider api.
	typeText(m, "/provider api")
	press(m, "enter")
	typeText(m, "chat")
	press(m, "enter")
	if _, ok := m.prov.(*openai.Provider); !ok {
		t.Fatalf("adapter = %T after /provider api", m.prov)
	}
}

func TestOpenAICanUseResponsesAPI(t *testing.T) {
	m, _ := newRegistryModel(t, secrets.Mem{"openai": "k"})
	typeText(m, "/provider openai")
	press(m, "enter")
	typeText(m, "/provider api")
	press(m, "enter")
	typeText(m, "responses")
	press(m, "enter")
	op, ok := m.prov.(*openai.Provider)
	if !ok || !op.Responses {
		t.Fatalf("adapter = %#v", m.prov)
	}
	if !strings.Contains(plainView(m), "OpenAI Responses") {
		t.Fatalf("switch notice missing:\n%s", plainView(m))
	}
}
