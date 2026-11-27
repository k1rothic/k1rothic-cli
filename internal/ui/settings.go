package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/core/permissions"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
	"github.com/akutor/k1rothic-cli/internal/ui/theme"
)

// setting is one row of the settings panel.
type setting struct {
	label  string
	value  func(m *Model) string
	choose func(m *Model, delta int) tea.Cmd // ←/→ (delta ±1) and Enter (delta 0 = open/next)
	note   func(m *Model) string             // optional muted text after the value
}

type settingsPanel struct {
	sel        int
	confirmAAP bool // second press needed to allow always-approve
	rows       []setting
}

func cycle(opts []string, cur string, delta int) string {
	i := slices.Index(opts, cur)
	if i < 0 {
		i = 0
	}
	if delta == 0 {
		delta = 1
	}
	return opts[(i+delta+len(opts))%len(opts)]
}

func newSettingsPanel() *settingsPanel {
	p := &settingsPanel{}
	p.rows = []setting{
		{
			label: "Provider",
			value: func(m *Model) string {
				if i, ok := registry.Lookup(m.cfg.Provider); ok {
					return i.Label
				}
				return m.cfg.Provider
			},
			choose: func(m *Model, _ int) tea.Cmd { return m.openProviderPicker() },
			note:   func(m *Model) string { return "Enter to change · keys live in the OS keychain" },
		},
		{
			label:  "Model",
			value:  func(m *Model) string { return m.cfg.Model },
			choose: func(m *Model, _ int) tea.Cmd { return m.openModelPicker() },
			note:   func(m *Model) string { return "Enter to change" },
		},
		{
			label:  "Custom endpoints",
			value:  func(m *Model) string { return itoaN(len(m.cfg.Endpoints)) + " saved" },
			choose: func(m *Model, _ int) tea.Cmd { return m.openEndpointsManager() },
			note:   func(m *Model) string { return "Enter to add, edit or delete" },
		},
		{
			label: "API format",
			value: func(m *Model) string { return m.apiLabel(m.cfg.Provider) },
			choose: func(m *Model, _ int) tea.Cmd {
				if len(registry.APIsFor(m.cfg.Provider)) < 2 {
					return nil
				}
				return m.openAPIPicker(m.cfg.Provider, func(m *Model) tea.Cmd { return m.switchProvider(m.cfg.Provider) })
			},
			note: func(m *Model) string {
				if len(registry.APIsFor(m.cfg.Provider)) < 2 {
					return "fixed for this provider"
				}
				return "Enter to choose · Chat Completions, Responses, Anthropic Messages, Gemini"
			},
		},
		{
			label: "Reasoning effort",
			value: func(m *Model) string { return m.cfg.Effort },
			choose: func(m *Model, d int) tea.Cmd {
				m.cfg.Effort = cycle([]string{"off", "low", "medium", "high"}, m.cfg.Effort, d)
				m.save()
				return nil
			},
		},
		{
			label: "Approval mode",
			value: func(m *Model) string { return m.approval.String() },
			choose: func(m *Model, d int) tea.Cmd {
				opts := []string{"ask", "auto-edit", "plan"}
				if m.allowAlways {
					opts = append(opts, "always-approve")
				}
				v := cycle(opts, m.approval.String(), d)
				mode, _ := permissions.Parse(v)
				m.approval = mode
				m.cfg.Approval.Mode = v
				m.save()
				return nil
			},
			note: func(m *Model) string { return "default for new sessions · Shift+Tab changes it for now" },
		},
		{
			label: "Allow always-approve",
			value: func(m *Model) string { return onOff(m.allowAlways) },
			choose: func(m *Model, _ int) tea.Cmd {
				if m.allowAlways {
					m.allowAlways = false
					m.cfg.Approval.AllowAlwaysApprove = false
					if m.approval == permissions.AlwaysApprove {
						m.approval = permissions.Ask
						m.cfg.Approval.Mode = permissions.Ask.String()
					}
					m.save()
					return nil
				}
				if !p.confirmAAP {
					p.confirmAAP = true
					return nil
				}
				p.confirmAAP = false
				m.allowAlways = true
				m.cfg.Approval.AllowAlwaysApprove = true
				m.save()
				return nil
			},
			note: func(m *Model) string {
				if p.confirmAAP {
					return m.th.Warn.Render(m.g.Warn + " tools would run without asking — press again to confirm")
				}
				return "adds always-approve to the Shift+Tab cycle"
			},
		},
		{
			label:  "Color theme",
			value:  func(m *Model) string { return m.cfg.UI.Color },
			choose: func(m *Model, _ int) tea.Cmd { return m.openThemePicker() },
			note:   func(m *Model) string { return "Enter to pick with live preview" },
		},
		{
			label: "Background",
			value: func(m *Model) string { return m.cfg.UI.Theme },
			choose: func(m *Model, d int) tea.Cmd {
				m.cfg.UI.Theme = cycle([]string{"auto", "dark", "light"}, m.cfg.UI.Theme, d)
				m.save()
				if m.cfg.UI.Theme == "auto" {
					return tea.RequestBackgroundColor
				}
				m.applyTheme(m.cfg.UI.Theme == "dark")
				return nil
			},
			note: func(m *Model) string { return "auto follows the terminal" },
		},
		{
			label: "Thinking animation",
			value: func(m *Model) string {
				return m.cfg.UI.Spinner + " " + strings.Join(ResolveSpinner(m.cfg.UI.Spinner).Frames[:4], "")
			},
			choose: func(m *Model, d int) tea.Cmd {
				m.cfg.UI.Spinner = cycle(SpinnerNames, m.cfg.UI.Spinner, d)
				m.setSpinner(m.cfg.UI.Spinner)
				m.save()
				return nil
			},
			note: func(m *Model) string { return "line = plain ASCII, if braille shows as boxes" },
		},
		{
			label: "Symbols",
			value: func(m *Model) string { return m.cfg.UI.Glyphs },
			choose: func(m *Model, d int) tea.Cmd {
				m.cfg.UI.Glyphs = cycle([]string{"auto", "unicode", "safe"}, m.cfg.UI.Glyphs, d)
				m.setGlyphs(ResolveGlyphs(m.cfg.UI.Glyphs))
				m.save()
				return nil
			},
			note: func(m *Model) string { return "safe = only symbols every font has" },
		},
		{
			label: "Mouse wheel scrolling",
			value: func(m *Model) string { return onOff(m.cfg.UI.Mouse) },
			choose: func(m *Model, _ int) tea.Cmd {
				m.cfg.UI.Mouse = !m.cfg.UI.Mouse
				m.save()
				return nil
			},
			note: func(m *Model) string { return "off = native text selection" },
		},
	}
	return p
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (p *settingsPanel) key(m *Model, k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "esc", "ctrl+c", "f2":
		return nil, true
	case "up", "k", "shift+tab":
		p.sel = (p.sel - 1 + len(p.rows)) % len(p.rows)
		p.confirmAAP = false
	case "down", "j", "tab":
		p.sel = (p.sel + 1) % len(p.rows)
		p.confirmAAP = false
	case "left", "h":
		return p.rows[p.sel].choose(m, -1), false
	case "right", "l", "space", " ", "enter":
		return p.rows[p.sel].choose(m, 1), false
	}
	return nil, false
}

func (p *settingsPanel) view(m *Model) ([]string, *tea.Cursor) {
	rows := []string{m.row(m.panelTitle("Settings"))}
	labelW := 0
	for _, r := range p.rows {
		labelW = max(labelW, ansi.StringWidth(r.label))
	}
	for i, r := range p.rows {
		label := r.label + strings.Repeat(" ", labelW-ansi.StringWidth(r.label))
		val := r.value(m)
		var line string
		if i == p.sel {
			line = m.th.Selected.Render(m.g.Selected+" "+label) + "  " + m.th.Bold.Render(val)
			if r.note != nil {
				line += "  " + m.th.Faint.Render(r.note(m))
			}
		} else {
			line = m.th.Text.Render("  "+label) + "  " + m.th.Muted.Render(val)
		}
		rows = append(rows, m.row(" "+line))
	}
	where := "not saved (no config path)"
	if m.opts.ConfigPath != "" {
		where = "saved to " + ShortenPath(m.opts.ConfigPath, m.home, max(m.contentWidth()-20, 10))
	}
	rows = append(rows, m.row(m.panelHint("↑/↓ or click", "←/→, Enter or click again to change", "Esc close", where)))
	return rows, nil
}

// openThemePicker lists the colour themes; moving the selection previews
// the theme live, Esc restores the previous one.
func (m *Model) openThemePicker() tea.Cmd {
	orig := m.cfg.UI.Color
	p := &picker{title: "Color theme"}
	for i, n := range theme.Names {
		d := theme.Label(n)
		if n == orig {
			d += " · current"
			p.sel = i
		}
		p.items = append(p.items, pickItem{value: n, label: n, detail: d})
	}
	p.onMove = func(m *Model, it pickItem) {
		m.cfg.UI.Color = it.value
		m.applyTheme(m.th.Dark)
	}
	p.onCancel = func(m *Model) {
		m.cfg.UI.Color = orig
		m.applyTheme(m.th.Dark)
	}
	p.onPick = func(m *Model, it pickItem) tea.Cmd {
		m.cfg.UI.Color = it.value
		m.applyTheme(m.th.Dark)
		m.save()
		m.notice("Color theme: "+it.value, false)
		return nil
	}
	m.panel = p
	return nil
}

func itoaN(n int) string { return strconv.Itoa(n) }
