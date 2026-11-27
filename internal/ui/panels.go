package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// panel is a modal popup shown above the input box (settings, pickers, key
// entry). While a panel is open it receives every key press.
type panel interface {
	// view returns full-width rows (already wrapped by m.row) and, for panels
	// with their own text field, the cursor relative to the first row.
	view(m *Model) (rows []string, cur *tea.Cursor)
	// key handles a key press. It returns close=true to dismiss the panel.
	key(m *Model, k tea.KeyPressMsg) (cmd tea.Cmd, close bool)
}

// panelTitle renders the "Title ─────" header row content.
func (m *Model) panelTitle(title string) string {
	t := " " + m.th.Accent.Render(title) + " "
	rest := max(m.contentWidth()-ansi.StringWidth(t)-1, 0)
	return m.th.BorderColor.Render(m.g.H) + t + m.th.BorderColor.Render(strings.Repeat(m.g.H, rest))
}

func (m *Model) panelHint(parts ...string) string {
	return "   " + m.th.Faint.Render(strings.Join(parts, " · "))
}

// ---- picker ----

type pickItem struct {
	value  string
	label  string
	detail string
}

// picker is a filterable list. Typing filters; Enter picks; Tab runs the
// optional alt action.
type picker struct {
	title    string
	items    []pickItem
	filter   string
	sel      int
	top      int
	loading  bool
	err      string
	custom   bool   // allow picking the filter text itself
	altHint  string // e.g. "Tab set key"
	onPick   func(m *Model, it pickItem) tea.Cmd
	onAlt    func(m *Model, it pickItem) tea.Cmd
	onMove   func(m *Model, it pickItem) // selection changed (live preview)
	onCancel func(m *Model)              // closed without picking
	tag      string                      // identifies the picker for async updates (e.g. "models:openai")
	emptyMsg string
}

func (p *picker) visible() []pickItem {
	var out []pickItem
	f := strings.ToLower(p.filter)
	for _, it := range p.items {
		if f == "" || strings.Contains(strings.ToLower(it.value+" "+it.label), f) {
			out = append(out, it)
		}
	}
	if p.custom && p.filter != "" {
		exact := false
		for _, it := range out {
			exact = exact || it.value == p.filter
		}
		if !exact {
			out = append(out, pickItem{value: p.filter, label: p.filter, detail: "use this id"})
		}
	}
	return out
}

func (p *picker) key(m *Model, k tea.KeyPressMsg) (tea.Cmd, bool) {
	vis := p.visible()
	moved := func() {
		if p.onMove != nil && p.sel < len(vis) {
			p.onMove(m, vis[p.sel])
		}
	}
	switch k.String() {
	case "esc", "ctrl+c":
		if p.onCancel != nil {
			p.onCancel(m)
		}
		return nil, true
	case "up", "ctrl+p":
		if len(vis) > 0 {
			p.sel = (p.sel - 1 + len(vis)) % len(vis)
			moved()
		}
	case "down", "ctrl+n":
		if len(vis) > 0 {
			p.sel = (p.sel + 1) % len(vis)
			moved()
		}
	case "pgup":
		p.sel = max(p.sel-maxPopupItems, 0)
	case "pgdown":
		p.sel = min(p.sel+maxPopupItems, max(len(vis)-1, 0))
	case "enter":
		if len(vis) == 0 || p.onPick == nil {
			return nil, false
		}
		return p.onPick(m, vis[p.sel]), true
	case "tab":
		if len(vis) == 0 || p.onAlt == nil {
			return nil, false
		}
		return p.onAlt(m, vis[p.sel]), true
	case "backspace":
		if p.filter != "" {
			r := []rune(p.filter)
			p.filter = string(r[:len(r)-1])
			p.sel = 0
		}
	default:
		if t := k.Text; t != "" && !strings.ContainsAny(t, "\r\n\t") {
			p.filter += t
			p.sel = 0
		}
	}
	return nil, false
}

func (p *picker) view(m *Model) ([]string, *tea.Cursor) {
	rows := []string{m.row(m.panelTitle(p.title))}
	if p.filter != "" {
		rows = append(rows, m.row("   "+m.th.Muted.Render("filter: ")+m.th.Text.Render(p.filter)))
	}
	vis := p.visible()
	if p.sel >= len(vis) {
		p.sel = max(len(vis)-1, 0)
	}
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+maxPopupItems {
		p.top = p.sel - maxPopupItems + 1
	}
	labelW := 0
	for _, it := range vis {
		labelW = max(labelW, ansi.StringWidth(it.label))
	}
	labelW = min(labelW, max(m.contentWidth()/2, 10))
	end := min(p.top+maxPopupItems, len(vis))
	for i := p.top; i < end; i++ {
		it := vis[i]
		label := ansi.Truncate(it.label, labelW, "…")
		label += strings.Repeat(" ", max(labelW-ansi.StringWidth(label), 0))
		var line string
		if i == p.sel {
			line = m.th.Selected.Render(m.g.Selected+" "+label) + "  " + m.th.Text.Render(it.detail)
		} else {
			line = m.th.Text.Render("  "+label) + "  " + m.th.Faint.Render(it.detail)
		}
		rows = append(rows, m.row(" "+line))
	}
	switch {
	case p.loading:
		rows = append(rows, m.row("   "+m.th.Muted.Render("Loading…")))
	case p.err != "":
		rows = append(rows, m.row("   "+m.th.Error.Render(m.g.Error+" "+p.err)))
	case len(vis) == 0:
		msg := p.emptyMsg
		if msg == "" {
			msg = "No matches."
		}
		rows = append(rows, m.row("   "+m.th.Muted.Render(msg)))
	}
	hints := []string{"↑/↓ or click", "Enter choose"}
	if p.altHint != "" {
		hints = append(hints, p.altHint)
	}
	hints = append(hints, "type to filter", "Esc close")
	if len(vis) > maxPopupItems {
		hints = append([]string{fmt.Sprintf("%d/%d", p.sel+1, len(vis))}, hints...)
	}
	rows = append(rows, m.row(m.panelHint(hints...)))
	return rows, nil
}

// ---- text prompt (API key / base URL) ----

type prompt struct {
	title      string
	help       []string // muted lines above the field
	input      textinput.Model
	onDone     func(m *Model, value string) tea.Cmd
	allowEmpty bool
}

func newPrompt(m *Model, title, label string, secret bool, help ...string) *prompt {
	ti := textinput.New()
	ti.Prompt = label + " "
	ti.CharLimit = 4096
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	st := textinput.DefaultStyles(m.th.Dark)
	for _, s := range []*textinput.StyleState{&st.Focused, &st.Blurred} {
		s.Prompt = m.th.Bold
		s.Text = lipgloss.NewStyle()
		s.Placeholder = m.th.Faint
	}
	ti.SetStyles(st)
	ti.SetVirtualCursor(false)
	ti.SetWidth(max(m.contentWidth()-8-ansi.StringWidth(ti.Prompt), 10))
	ti.Focus()
	return &prompt{title: title, help: help, input: ti}
}

func (p *prompt) key(m *Model, k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "esc", "ctrl+c":
		return nil, true
	case "enter":
		v := strings.TrimSpace(p.input.Value())
		if v == "" && !p.allowEmpty {
			return nil, false
		}
		p.input.Reset() // don't keep secrets around longer than needed
		return p.onDone(m, v), true
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(k)
	return cmd, false
}

func (p *prompt) view(m *Model) ([]string, *tea.Cursor) {
	rows := []string{m.row(m.panelTitle(p.title))}
	for _, h := range p.help {
		for _, l := range strings.Split(ansi.Wrap(h, max(m.contentWidth()-4, 10), ""), "\n") {
			rows = append(rows, m.row("   "+m.th.Muted.Render(l)))
		}
	}
	fieldRow := len(rows)
	rows = append(rows, m.row("   "+p.input.View()))
	rows = append(rows, m.row(m.panelHint("Enter save", "Esc cancel")))
	var cur *tea.Cursor
	if c := p.input.Cursor(); c != nil {
		c.X += m.padX() + 3
		c.Y = fieldRow
		cur = c
	}
	return rows, cur
}

// update forwards a message (bracketed paste, Ctrl+V clipboard text, blink)
// to the field.
func (p *prompt) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}
