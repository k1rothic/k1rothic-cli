package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/permissions"
	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/fake"
)

// newTestModel builds a sized model backed by an instant fake provider.
func newTestModel(t *testing.T, w, h int, p provider.Provider) *Model {
	t.Helper()
	cfg := testConfig()
	if p == nil {
		p = &fake.Provider{Reply: func(provider.Request) (string, string) {
			return "pondering", "## Answer\n\nHello **world**."
		}}
	}
	m := New(Options{Config: cfg, Provider: p, Cwd: t.TempDir(), ContextLimit: 500_000})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Provider, cfg.Model, cfg.Effort = "fake", "fake-1", "low"
	cfg.UI.Theme = "dark"
	return cfg
}

func press(m *Model, s string) tea.Cmd {
	var k tea.KeyPressMsg
	switch s {
	case "enter":
		k = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		k = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		k = tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		k = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "shift+enter":
		k = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	case "ctrl+c":
		k = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+o":
		k = tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	default:
		k = tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
	_, cmd := m.Update(k)
	return cmd
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// drain runs the stream commands to completion synchronously.
func drain(t *testing.T, m *Model) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for m.streaming {
		if time.Now().After(deadline) {
			t.Fatal("stream did not finish")
		}
		msg := waitEvents(m.streamID, m.events)()
		m.Update(msg)
	}
}

func plainView(m *Model) string { return ansi.Strip(m.View().Content) }

func TestLayoutFillsScreenExactly(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 12}, {20, 8}} {
		m := newTestModel(t, size[0], size[1], nil)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: view has %d lines", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Errorf("%dx%d: line %d width %d: %q", size[0], size[1], i, w, ansi.Strip(l))
			}
		}
	}
}

func TestOuterPadding(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	typeText(m, "hello")
	press(m, "enter")
	drain(t, m)
	for i, l := range strings.Split(plainView(m), "\n") {
		if l != "" && !strings.HasPrefix(l, "  ") {
			t.Errorf("row %d lacks left padding: %q", i, l)
		}
	}
	// Assistant text and the user prompt share the same inset.
	v := plainView(m)
	if !strings.Contains(v, strings.Repeat(" ", 2+indent)+m.g.Prompt+" hello") {
		t.Errorf("user prompt not inset:\n%s", v)
	}
	if !strings.Contains(v, strings.Repeat(" ", 2+indent)+"Answer") {
		t.Errorf("assistant heading not inset:\n%s", v)
	}
	if strings.Contains(v, "## Answer") {
		t.Error("headings must render without ## prefix")
	}
}

func TestSendStreamsAndRenders(t *testing.T) {
	m := newTestModel(t, 100, 30, nil)
	typeText(m, "hi there")
	press(m, "enter")
	if !m.streaming {
		t.Fatal("expected streaming after enter")
	}
	if !strings.Contains(plainView(m), "Responding") && !strings.Contains(plainView(m), "Thinking") {
		t.Fatalf("no status line:\n%s", plainView(m))
	}
	drain(t, m)

	v := plainView(m)
	for _, want := range []string{"hi there", "Thought for", "Answer", "world", "fake-1 (low) · ask"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "pondering") {
		t.Error("reasoning should be collapsed by default")
	}
	press(m, "ctrl+o")
	if !strings.Contains(plainView(m), "pondering") {
		t.Error("ctrl+o should expand reasoning")
	}
	if len(m.history) != 2 || m.history[1].Role != provider.RoleAssistant {
		t.Fatalf("history = %+v", m.history)
	}
	if m.ctxUsed == 0 {
		t.Error("token counter not updated from usage")
	}
}

func TestCancelStream(t *testing.T) {
	p := &fake.Provider{Delay: 50 * time.Millisecond}
	m := newTestModel(t, 80, 24, p)
	typeText(m, "go")
	press(m, "enter")
	m.Update(waitEvents(m.streamID, m.events)()) // first batch
	press(m, "esc")
	drain(t, m)
	if !strings.Contains(plainView(m), "Interrupted") {
		t.Fatalf("expected interrupted marker:\n%s", plainView(m))
	}
}

func TestShiftEnterInsertsNewline(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	typeText(m, "a")
	press(m, "shift+enter")
	typeText(m, "b")
	if got := m.input.Value(); got != "a\nb" {
		t.Fatalf("input = %q", got)
	}
	if m.streaming {
		t.Fatal("shift+enter must not send")
	}
}

func TestShiftTabCyclesApproval(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	want := []permissions.Mode{permissions.AutoEdit, permissions.Plan, permissions.Ask}
	for _, w := range want {
		press(m, "shift+tab")
		if m.approval != w {
			t.Fatalf("approval = %v, want %v", m.approval, w)
		}
	}
}

func TestSlashCompletionAndCommands(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	typeText(m, "/ch")
	if !strings.Contains(plainView(m), "/chat") {
		t.Fatalf("completion popup missing:\n%s", plainView(m))
	}
	press(m, "enter")
	if m.toolsOn {
		t.Fatal("/chat should turn tools off")
	}
	typeText(m, "/agent")
	press(m, "enter")
	if !m.toolsOn {
		t.Fatal("/agent should turn tools on")
	}
	typeText(m, "/cost")
	press(m, "enter")
	if !strings.Contains(plainView(m), "M4") {
		t.Fatal("unimplemented command should mention its milestone")
	}
	typeText(m, "/bogus x")
	press(m, "enter")
	if !strings.Contains(plainView(m), "Unknown command /bogus") {
		t.Fatal("unknown command not reported")
	}
}

func TestCtrlCDoublePressQuits(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	// The first press returns a 2s disarm tick; don't run it here.
	if cmd := press(m, "ctrl+c"); cmd == nil {
		t.Fatal("expected disarm tick")
	}
	if m.quitArmedAt.IsZero() {
		t.Fatal("first ctrl+c should arm quit")
	}
	if !strings.Contains(plainView(m), "Ctrl+C again") {
		t.Fatal("missing exit hint")
	}
	cmd := press(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("second ctrl+c should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("second ctrl+c should return tea.Quit")
	}
}

func TestCtrlCClearsInputFirst(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	typeText(m, "draft")
	press(m, "ctrl+c")
	if m.input.Value() != "" {
		t.Fatal("ctrl+c should clear input")
	}
	if !m.quitArmedAt.IsZero() {
		t.Fatal("clearing input must not arm quit")
	}
}

func TestPasteNormalizesCRLF(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	m.Update(tea.PasteMsg{Content: "line1\r\nline2\rline3"})
	if got := m.input.Value(); got != "line1\nline2\nline3" {
		t.Fatalf("input = %q", got)
	}
}

func TestStaleStreamIgnored(t *testing.T) {
	m := newTestModel(t, 80, 24, nil)
	m.Update(streamMsg{id: 99, events: []provider.Event{{Kind: provider.EventTextDelta, Text: "ghost"}}})
	if strings.Contains(plainView(m), "ghost") {
		t.Fatal("stale stream rendered")
	}
}

func TestProviderErrorShown(t *testing.T) {
	p := errProvider{}
	m := newTestModel(t, 80, 24, p)
	typeText(m, "x")
	press(m, "enter")
	drain(t, m)
	if !strings.Contains(plainView(m), "boom") {
		t.Fatalf("error not shown:\n%s", plainView(m))
	}
}

type errProvider struct{}

func (errProvider) Name() string { return "err" }
func (errProvider) Stream(context.Context, provider.Request) <-chan provider.Event {
	ch := make(chan provider.Event, 1)
	ch <- provider.Event{Kind: provider.EventError, Err: errBoom{}}
	close(ch)
	return ch
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }

// wgl4Safe is ASCII plus the WGL4 symbols the safe glyph set and our markdown
// style may emit. WGL4 is covered by practically every terminal font.
const wgl4Safe = "─│┌┐└┘▼▐♦›·…↓↑←→►█▀▄•×— "

func TestSafeGlyphsOnlyEmitWGL4(t *testing.T) {
	cfg := testConfig()
	cfg.UI.Glyphs = "safe"
	cfg.UI.Spinner = "line" // braille is opt-out, not part of the safe set
	p := &fake.Provider{Delay: time.Millisecond}
	m := New(Options{Config: cfg, Provider: p, Cwd: t.TempDir(), ContextLimit: 500_000})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	check := func(stage string) {
		for _, r := range plainView(m) {
			if r > 127 && !strings.ContainsRune(wgl4Safe, r) {
				t.Errorf("%s: non-WGL4 rune %q (U+%04X) in safe mode", stage, r, r)
			}
		}
	}
	typeText(m, "hi")
	press(m, "enter")
	m.Update(waitEvents(m.streamID, m.events)())
	check("streaming")
	drain(t, m)
	check("done")
	press(m, "ctrl+o")
	check("expanded")
	typeText(m, "/")
	check("completion")
	press(m, "esc")
	m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	check("help")
	m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	check("settings")
	press(m, "esc")
	typeText(m, "/model")
	press(m, "enter")
	check("model picker")
}
