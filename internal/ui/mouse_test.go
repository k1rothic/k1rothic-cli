package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/fake"
	"github.com/akutor/k1rothic-cli/internal/secrets"
)

// find returns the screen cell of the first occurrence of s.
func find(t *testing.T, m *Model, s string) (x, y int) {
	t.Helper()
	for i, l := range viewLines(m) {
		if j := strings.Index(l, s); j >= 0 {
			return ansi.StringWidth(l[:j]), i
		}
	}
	t.Fatalf("%q not on screen:\n%s", s, plainView(m))
	return 0, 0
}

func clickAt(m *Model, x, y int) tea.Cmd {
	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return cmd
}

func TestClickThoughtToggles(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	typeText(m, "hi")
	press(m, "enter")
	drain(t, m)
	x, y := find(t, m, "Thought for")
	clickAt(m, x, y)
	if !strings.Contains(plainView(m), "pondering") {
		t.Fatalf("click should open reasoning:\n%s", plainView(m))
	}
	x, y = find(t, m, "Thought for")
	clickAt(m, x, y)
	if strings.Contains(plainView(m), "pondering") {
		t.Fatal("second click should close reasoning")
	}
}

func TestLongPromptFoldsAndOpens(t *testing.T) {
	m := newTestModel(t, 100, 40, nil)
	var lines []string
	for i := range 20 {
		lines = append(lines, "line"+itoa2(i))
	}
	m.Update(tea.PasteMsg{Content: strings.Join(lines, "\n")})
	press(m, "enter")
	drain(t, m)
	v := plainView(m)
	if strings.Contains(v, "line10") || !strings.Contains(v, "16 more lines") {
		t.Fatalf("long prompt not folded:\n%s", v)
	}
	if m.history[0].Text != strings.Join(lines, "\n") {
		t.Fatal("folding must not change what is sent")
	}
	x, y := find(t, m, "more lines")
	clickAt(m, x, y)
	if !strings.Contains(plainView(m), "show less") {
		t.Fatalf("click should unfold:\n%s", plainView(m))
	}
	x, y = find(t, m, "show less")
	clickAt(m, x, y)
	if !strings.Contains(plainView(m), "more lines") {
		t.Fatal("click should fold again")
	}
	press(m, "ctrl+o") // opens everything
	if !strings.Contains(plainView(m), "show less") || !strings.Contains(plainView(m), "pondering") {
		t.Fatal("ctrl+o should open prompts and reasoning")
	}
	press(m, "ctrl+o")
	if strings.Contains(plainView(m), "show less") || strings.Contains(plainView(m), "pondering") {
		t.Fatal("second ctrl+o should close everything")
	}
}

func itoa2(i int) string { return string(rune('0'+i/10)) + string(rune('0'+i%10)) }

func TestThinkingAnimationBeforeFirstToken(t *testing.T) {
	block := make(chan struct{})
	p := &blockingProvider{release: block}
	m := newTestModel(t, 100, 30, p)
	defer close(block)
	typeText(m, "hi")
	press(m, "enter")
	frame0 := ansi.Strip(m.sb.spinFrame)
	if !strings.Contains(plainView(m), "Thinking") || !strings.Contains(plainView(m), frame0) {
		t.Fatalf("no thinking animation:\n%s", plainView(m))
	}
	if !strings.ContainsAny(frame0, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		t.Fatalf("default spinner should be braille, got %q", frame0)
	}
	m.Update(m.spin.Tick())
	if ansi.Strip(m.sb.spinFrame) == frame0 {
		t.Fatal("spinner did not advance")
	}
}

type blockingProvider struct{ release chan struct{} }

func (blockingProvider) Name() string { return "block" }
func (b *blockingProvider) Stream(ctx context.Context, _ provider.Request) <-chan provider.Event {
	ch := make(chan provider.Event)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
		case <-b.release:
		}
	}()
	return ch
}

func TestThemePickerPreviewsAndReverts(t *testing.T) {
	m, path := newRegistryModel(t, secrets.Mem{})
	typeText(m, "/theme")
	press(m, "enter")
	if _, ok := m.panel.(*picker); !ok {
		t.Fatalf("panel = %T", m.panel)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.th.Name != "graphite" {
		t.Fatalf("preview theme = %q", m.th.Name)
	}
	press(m, "esc")
	if m.th.Name != "carbon" || m.cfg.UI.Color != "carbon" {
		t.Fatalf("esc should revert, got %q", m.th.Name)
	}
	typeText(m, "/theme forest")
	press(m, "enter")
	got, _ := config.Load(path)
	if m.th.Name != "forest" || got.UI.Color != "forest" {
		t.Fatalf("theme not applied/saved: %q %q", m.th.Name, got.UI.Color)
	}
}

func TestClickFooterAndPicker(t *testing.T) {
	m, _ := newRegistryModel(t, secrets.Mem{})
	x, y := find(t, m, "F2:settings")
	clickAt(m, x+1, y)
	if _, ok := m.panel.(*settingsPanel); !ok {
		t.Fatalf("footer click should open settings, panel = %T", m.panel)
	}
	// Click "Color theme" twice: select, then open the picker.
	x, y = find(t, m, "Color theme")
	clickAt(m, x, y)
	clickAt(m, x, y)
	if _, ok := m.panel.(*picker); !ok {
		t.Fatalf("panel = %T", m.panel)
	}
	x, y = find(t, m, "amber")
	clickAt(m, x, y)
	if m.panel != nil || m.cfg.UI.Color != "amber" {
		t.Fatalf("click should pick: panel=%T color=%q", m.panel, m.cfg.UI.Color)
	}
}

func TestClickInInputMovesCursor(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	typeText(m, "hello world")
	x, y := find(t, m, "world")
	clickAt(m, x, y)
	if m.input.Column() != 6 {
		t.Fatalf("cursor column = %d, want 6", m.input.Column())
	}
	typeText(m, "big ")
	if m.input.Value() != "hello big world" {
		t.Fatalf("value = %q", m.input.Value())
	}
}

func TestClickStopCancels(t *testing.T) {
	m := newTestModel(t, 100, 30, &fake.Provider{Delay: 50 * time.Millisecond})
	typeText(m, "go")
	press(m, "enter")
	m.Update(waitEvents(m.streamID, m.events)())
	x, y := find(t, m, "[stop]")
	clickAt(m, x+1, y)
	drain(t, m)
	if !strings.Contains(plainView(m), "Interrupted") {
		t.Fatalf("[stop] click should cancel:\n%s", plainView(m))
	}
}

func TestThemesPaintBackgroundAndRestoreOnQuit(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	if m.View().BackgroundColor != nil {
		t.Fatal("must not paint before the terminal reported its own background")
	}
	orig := lipgloss.Color("#300A24")
	m.Update(tea.BackgroundColorMsg{Color: orig})
	carbon := m.View().BackgroundColor
	if carbon == nil {
		t.Fatal("carbon should paint the window")
	}
	for _, name := range []string{"graphite", "ocean", "mono"} {
		m.cfg.UI.Color = name
		m.applyTheme(true)
		bg := m.View().BackgroundColor
		if bg == nil || bg == carbon {
			t.Fatalf("%s should paint its own background, got %v", name, bg)
		}
	}
	cmd := m.quit()
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit should return tea.Quit")
	}
	if m.View().BackgroundColor != orig {
		t.Fatal("final frame must restore the terminal's original background")
	}
}
