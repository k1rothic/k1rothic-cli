package ui

import (
	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
)

// layout records where the last View placed each region, so mouse clicks
// can be mapped back to what was clicked.
type layout struct {
	vpTop     int // first scrollback row
	statusY   int
	popupTop  int
	popupRows int
	boxTop    int // top border of the input box
	footerY   int
}

// clicker is implemented by panels whose rows react to clicks. row is
// relative to the panel's first row.
type clicker interface {
	click(m *Model, row int) (cmd tea.Cmd, close bool)
}

const footerSep = "  │  "

type footerHint struct {
	key, desc string
	action    func(m *Model) tea.Cmd
}

func (m *Model) footerHints() []footerHint {
	help := footerHint{"Ctrl+.", "shortcuts", func(m *Model) tea.Cmd { m.showHelp = !m.showHelp; return nil }}
	if m.width < narrowWidth {
		return []footerHint{help}
	}
	return []footerHint{
		{"Shift+Tab", "mode", func(m *Model) tea.Cmd { m.approval = m.approval.Next(m.allowAlways); return nil }},
		{"Ctrl+c", "cancel", func(m *Model) tea.Cmd { m.cancelStream(); return nil }},
		{"F2", "settings", func(m *Model) tea.Cmd { m.openSettings(); return nil }},
		help,
	}
}

func (m *Model) openSettings() {
	m.showHelp = false
	m.panel = newSettingsPanel()
}

// click handles a left click at screen cell (x, y).
func (m *Model) click(x, y int) tea.Cmd {
	defer m.relayout(false)
	l := m.lay
	switch {
	case y >= l.vpTop && y < l.vpTop+m.vp.Height():
		// Thinking headers and folded prompts open/close on click.
		if m.welcomeHot != nil {
			if cmd, ok := m.welcomeHot[m.vp.YOffset()+(y-l.vpTop)]; ok {
				name, args, _ := parseSlash(cmd)
				return m.runCommand(name, args)
			}
			return nil
		}
		line := m.vp.YOffset() + (y - l.vpTop) - m.padTop
		if e := m.sb.hotspotAt(line); e != nil {
			m.sb.toggle(e)
			m.refresh()
		}
		return nil

	case y == l.statusY && m.streaming && x >= m.width-m.padX()-len("[stop]")-1:
		m.cancelStream()
		return nil

	case l.popupRows > 0 && y >= l.popupTop && y < l.popupTop+l.popupRows:
		row := y - l.popupTop
		if m.panel != nil {
			c, ok := m.panel.(clicker)
			if !ok {
				return nil
			}
			cur := m.panel
			cmd, closed := c.click(m, row)
			if closed && m.panel == cur {
				m.panel = nil
			}
			return cmd
		}
		if matches := m.completions(); row < len(matches) && !m.showHelp {
			name := matches[row].Name
			m.input.Reset()
			m.compSel = 0
			return m.runCommand(name, "")
		}
		return nil

	case m.panel == nil && y > l.boxTop && y < l.boxTop+1+m.input.Height():
		m.placeInputCursor(x, y-l.boxTop-1)
		return nil

	case y == l.footerY:
		x0 := m.padX()
		for _, h := range m.footerHints() {
			w := ansi.StringWidth(h.key + ":" + h.desc)
			if x >= x0 && x < x0+w {
				return h.action(m)
			}
			x0 += w + ansi.StringWidth(footerSep)
		}
	}
	return nil
}

// placeInputCursor moves the text cursor to the clicked visual row/column of
// the input box.
func (m *Model) placeInputCursor(x, visualRow int) {
	if visualRow < 0 {
		visualRow = 0
	}
	if visualRow >= m.input.Height() {
		visualRow = m.input.Height() - 1
	}
	col := x - m.padX() - 2 - 2 // "│ " + prompt
	m.input.MoveToBegin()
	for range visualRow + m.input.ScrollYOffset() {
		m.input.CursorDown()
	}
	m.input.SetCursorColumn(m.input.LineInfo().StartColumn + max(col, 0))
}

// ---- panel click handlers ----

func (p *picker) click(m *Model, row int) (tea.Cmd, bool) {
	first := 1 // title row
	if p.filter != "" {
		first++
	}
	vis := p.visible()
	i := p.top + row - first
	if row < first || i < 0 || i >= len(vis) || i >= p.top+maxPopupItems {
		return nil, false
	}
	p.sel = i
	if p.onMove != nil {
		p.onMove(m, vis[i])
	}
	if p.onPick == nil {
		return nil, false
	}
	return p.onPick(m, vis[i]), true
}

func (p *settingsPanel) click(m *Model, row int) (tea.Cmd, bool) {
	i := row - 1 // title row
	if i < 0 || i >= len(p.rows) {
		return nil, false
	}
	if i != p.sel {
		p.sel = i
		p.confirmAAP = false
		return nil, false
	}
	return p.rows[i].choose(m, 1), false
}
