package ui

import (
	"math/rand"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/secrets"
)

func TestContextMeter(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	m.ctxUsed = 112_500 // of 500K
	top := viewLines(m)[1]
	for _, want := range []string{"22.5%", "113K / 500K", "███"} {
		if !strings.Contains(top, want) {
			t.Fatalf("top bar missing %q: %q", want, top)
		}
	}
}

func TestWelcomeScreenAndClick(t *testing.T) {
	m := newTestModel(t, 100, 36, nil)
	v := plainView(m)
	for _, want := range []string{"k1rothic", "/provider", "/theme", "/endpoints", "Type a message to start"} {
		if !strings.Contains(v, want) {
			t.Fatalf("welcome missing %q:\n%s", want, v)
		}
	}
	x, y := find(t, m, "/theme")
	clickAt(m, x, y)
	if _, ok := m.panel.(*picker); !ok {
		t.Fatalf("clicking /theme should open the theme picker, panel = %T", m.panel)
	}
	press(m, "esc")
	typeText(m, "hi")
	press(m, "enter")
	drain(t, m)
	if strings.Contains(plainView(m), "Type a message to start") {
		t.Fatal("welcome must disappear once the chat starts")
	}
	press(m, "esc")
	typeText(m, "/clear")
	press(m, "enter")
	if !strings.Contains(plainView(m), "Type a message to start") {
		t.Fatal("/clear should bring the welcome screen back")
	}
}

func TestWelcomeFitsTinyTerminals(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {60, 18}, {120, 50}} {
		m := newTestModel(t, size[0], size[1], nil)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) != size[1] {
			t.Errorf("%v: %d rows", size, len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != size[0] {
				t.Errorf("%v: row %d width %d", size, i, w)
			}
		}
	}
}

// TestFrameAlwaysFitsScreen drives random UI actions and checks every frame
// has exactly the terminal's height and width (a taller frame scrolls the
// screen and leaves ghost rows, e.g. a duplicated input border).
func TestFrameAlwaysFitsScreen(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	m, _ := newRegistryModel(t, secrets.Mem{})
	actions := []func(){
		func() { typeText(m, "/") },
		func() { typeText(m, "/se") },
		func() { typeText(m, "abc") },
		func() { press(m, "enter") },
		func() { press(m, "esc") },
		func() { press(m, "ctrl+c") },
		func() { press(m, "shift+enter") },
		func() { m.Update(tea.KeyPressMsg{Code: tea.KeyF1}) },
		func() { m.Update(tea.KeyPressMsg{Code: tea.KeyF2}) },
		func() { m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) },
		func() { m.Update(tea.KeyPressMsg{Code: tea.KeyUp}) },
		func() { m.Update(tea.PasteMsg{Content: "line1\nline2\nline3\nline4"}) },
		func() { typeText(m, "/model") },
		func() { typeText(m, "/theme") },
		func() { m.Update(tea.WindowSizeMsg{Width: 60 + r.Intn(80), Height: 14 + r.Intn(30)}) },
		func() {
			if m.streaming {
				drain(t, m)
			}
		},
	}
	for i := range 600 {
		actions[r.Intn(len(actions))]()
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) != m.height {
			t.Fatalf("step %d: frame has %d rows, terminal %d:\n%s", i, len(lines), m.height, plainView(m))
		}
		for j, l := range lines {
			if w := ansi.StringWidth(l); w != m.width {
				t.Fatalf("step %d: row %d width %d, terminal %d: %q", i, j, w, m.width, ansi.Strip(l))
			}
		}
	}
}

func TestPaintKeepsBackgroundAfterResets(t *testing.T) {
	seq := bgSeq(lipgloss.Color("#151515"))
	if seq != "\x1b[48;2;21;21;21m" {
		t.Fatalf("seq = %q", seq)
	}
	got := withBg("a\x1b[1mb\x1b[0mc\x1b[38;5;0md\x1b[mE\x1b[49mf", seq)
	want := "a\x1b[1mb\x1b[0m" + seq + "c\x1b[38;5;0md\x1b[m" + seq + "E\x1b[49m" + seq + "f"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if clearsBg("38;2;0;0;0") || clearsBg("48;5;0") || !clearsBg("1;0") {
		t.Fatal("clearsBg misreads colour arguments")
	}
	m := newTestModel(t, 60, 20, nil)
	if !strings.Contains(m.View().Content, seq) {
		t.Fatal("carbon should paint every row with its background")
	}
}

func TestMultipleEndpointsAddEditDelete(t *testing.T) {
	store := secrets.Mem{}
	m, path := newRegistryModel(t, store)
	add := func(name, apiFilter, url, key string) {
		t.Helper()
		typeText(m, "/endpoints")
		press(m, "enter")
		typeText(m, "add")
		press(m, "enter") // "+ Add custom endpoint"
		typeText(m, name)
		press(m, "enter")
		typeText(m, apiFilter)
		press(m, "enter")
		m.Update(tea.PasteMsg{Content: url})
		press(m, "enter")
		m.Update(tea.PasteMsg{Content: key})
		press(m, "enter")
		if _, ok := m.panel.(*picker); ok { // model picker for a new endpoint
			typeText(m, "model-"+name)
			press(m, "enter")
		}
	}
	add("local", "chat", "http://localhost:11434/v1", "")
	add("deepseek", "anthropic", "https://api.deepseek.com/anthropic", "ds-key")
	if len(m.cfg.Endpoints) != 2 || m.cfg.Provider != "custom:deepseek" || store["custom:deepseek"] != "ds-key" {
		t.Fatalf("endpoints=%v provider=%s store=%v", m.cfg.Endpoints, m.cfg.Provider, store)
	}
	got, _ := config.Load(path)
	if got.Endpoints["local"].API != "chat-completions" || got.Endpoints["deepseek"].API != "anthropic-messages" {
		t.Fatalf("saved %+v", got.Endpoints)
	}

	// Both appear in the provider list; switching back works.
	typeText(m, "/provider")
	press(m, "enter")
	v := plainView(m)
	if !strings.Contains(v, "local") || !strings.Contains(v, "deepseek") || !strings.Contains(v, "+ Add custom endpoint") {
		t.Fatalf("provider list:\n%s", v)
	}
	press(m, "esc")
	typeText(m, "/provider local")
	press(m, "enter")
	if m.cfg.Provider != "custom:local" || m.cfg.Model != "model-local" {
		t.Fatalf("provider=%s model=%s", m.cfg.Provider, m.cfg.Model)
	}

	// Edit: change deepseek's URL from its menu.
	typeText(m, "/endpoints")
	press(m, "enter")
	typeText(m, "deepseek")
	press(m, "enter")
	typeText(m, "url")
	press(m, "enter")
	p := m.panel.(*prompt)
	p.input.SetValue("")
	m.Update(tea.PasteMsg{Content: "https://gw2.example/anthropic"})
	press(m, "enter")
	if m.cfg.Endpoints["deepseek"].BaseURL != "https://gw2.example/anthropic" {
		t.Fatalf("url = %q", m.cfg.Endpoints["deepseek"].BaseURL)
	}

	// Delete the current endpoint (local): needs confirmation.
	typeText(m, "/endpoints")
	press(m, "enter")
	typeText(m, "local")
	press(m, "enter")
	typeText(m, "delete")
	press(m, "enter")
	if !strings.Contains(plainView(m), "Delete endpoint local?") {
		t.Fatalf("no confirmation:\n%s", plainView(m))
	}
	press(m, "enter") // default is "No, keep it"
	if _, ok := m.cfg.Endpoints["local"]; !ok {
		t.Fatal("'No' must keep the endpoint")
	}
	typeText(m, "delete")
	press(m, "enter")
	typeText(m, "yes")
	press(m, "enter")
	if _, ok := m.cfg.Endpoints["local"]; ok {
		t.Fatal("endpoint not deleted")
	}
	if m.cfg.Provider != "anthropic" {
		t.Fatalf("deleting the current endpoint should fall back, provider = %s", m.cfg.Provider)
	}
	got, _ = config.Load(path)
	if _, ok := got.Endpoints["local"]; ok || got.Endpoints["deepseek"].BaseURL == "" {
		t.Fatalf("saved %+v", got.Endpoints)
	}
}

func TestEndpointNameValidation(t *testing.T) {
	m, _ := newRegistryModel(t, secrets.Mem{})
	typeText(m, "/provider custom")
	press(m, "enter")
	typeText(m, "bad name!")
	press(m, "enter")
	if _, ok := m.panel.(*prompt); !ok || len(m.cfg.Endpoints) != 0 {
		t.Fatalf("invalid name should re-ask; panel=%T endpoints=%v", m.panel, m.cfg.Endpoints)
	}
}
