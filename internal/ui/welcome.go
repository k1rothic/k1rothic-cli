package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
)

// ---- context meter (top bar) ----

// contextMeter renders "▐████░░░░░░ 22.5%  223 / 500K": a small bar of the
// context window in use, the percentage, and used / total tokens.
func (m *Model) contextMeter() string {
	used := m.ctxUsed
	if m.streaming && m.turnUsage.Total() > 0 {
		used = m.turnUsage.Total()
	}
	if m.ctxLimit <= 0 {
		return m.th.Text.Render(FormatTokens(used)) + m.th.Muted.Render(" tokens")
	}
	frac := math.Min(float64(used)/float64(m.ctxLimit), 1)
	barW := 10
	if m.width < narrowWidth {
		barW = 5
	}
	filled := int(math.Round(frac * float64(barW)))
	if used > 0 && filled == 0 {
		filled = 1 // show that something is in use
	}
	fill := m.th.Accent
	switch {
	case frac >= 0.85:
		fill = m.th.Error
	case frac >= 0.6:
		fill = m.th.Warn
	}
	bar := fill.Render(strings.Repeat("█", filled)) + m.th.BorderColorBlur.Render(strings.Repeat("█", barW-filled))
	pct := fmt.Sprintf("%.1f%%", frac*100)
	return bar + " " + m.th.Text.Render(pct) + "  " +
		m.th.Text.Render(FormatTokens(used)) + m.th.Muted.Render(" / "+FormatTokens(m.ctxLimit))
}

// ---- welcome screen (new chat) ----

// isNewChat reports whether the conversation has not started yet.
func (m *Model) isNewChat() bool {
	for _, e := range m.sb.entries {
		if e.kind == entryUser || e.kind == entryAssistant {
			return false
		}
	}
	return true
}

type welcomeItem struct{ cmd, desc string }

var welcomeItems = []welcomeItem{
	{"/provider", "switch provider or add a custom endpoint"},
	{"/model", "pick a model"},
	{"/theme", "color theme"},
	{"/endpoints", "manage custom API endpoints"},
	{"/settings", "all settings (F2)"},
	{"/help", "commands and shortcuts"},
}

// welcomeContent is the centred logo + menu shown instead of an empty
// scrollback. Menu rows are clickable (m.welcomeHot maps content line →
// command).
func (m *Model) welcomeContent() string {
	cw := m.contentWidth()
	h := m.vp.Height()
	m.welcomeHot = map[int]string{}

	var block []string // rendered lines, unpadded
	type hot struct {
		line int
		cmd  string
	}
	var hots []hot

	logo := m.logoLines()
	sub := m.providerLabel(m.cfg.Provider) + " · " + m.cfg.Model
	if registry.IsEndpoint(m.cfg.Provider) {
		sub = m.providerLabel(m.cfg.Provider) + " (" + m.apiLabel(m.cfg.Provider) + ") · " + m.cfg.Model
	}
	title := m.th.Bold.Render("k1rothic")
	if v := m.opts.Version; v != "" {
		title += m.th.Faint.Render(" " + v)
	}
	center := func(s string) string {
		pad := max((cw-ansi.StringWidth(s))/2, 0)
		return strings.Repeat(" ", pad) + s
	}

	cmdW := 0
	for _, it := range welcomeItems {
		cmdW = max(cmdW, len(it.cmd))
	}
	menuW := 0
	var menu []string
	for _, it := range welcomeItems {
		l := m.th.Accent.Render(fmt.Sprintf("%-*s", cmdW, it.cmd)) + "   " + m.th.Muted.Render(it.desc)
		menuW = max(menuW, ansi.StringWidth(l))
		menu = append(menu, l)
	}
	menuPad := strings.Repeat(" ", max((cw-menuW)/2, 0))

	notices := m.noticeLines()
	needed := len(logo) + 1 + 2 + 1 + len(menu) + 2 + len(notices)
	if needed > h {
		logo = nil // short terminal: drop the logo first
	}
	for _, l := range logo {
		block = append(block, center(l))
	}
	if len(logo) > 0 {
		block = append(block, "")
	}
	block = append(block, center(title), center(m.th.Muted.Render(sub)), "")
	for i, l := range menu {
		hots = append(hots, hot{len(block), welcomeItems[i].cmd})
		block = append(block, menuPad+l)
	}
	block = append(block, "", center(m.th.Faint.Render("Type a message to start · click a command or type it")))
	if len(notices) > 0 {
		block = append(block, "")
		block = append(block, notices...)
	}

	top := max((h-len(block))/2, 0)
	lines := make([]string, top, top+len(block))
	lines = append(lines, block...)
	for _, hs := range hots {
		m.welcomeHot[top+hs.line] = hs.cmd
	}
	m.padTop = 0
	return strings.Join(lines, "\n")
}

// noticeLines renders the notices/errors added before the chat started
// (missing key, switched provider, …) below the menu.
func (m *Model) noticeLines() []string {
	var out []string
	for _, e := range m.sb.entries {
		if e.kind == entryNotice || e.kind == entryError {
			text, _ := m.sb.renderEntry(e)
			out = append(out, strings.Split(text, "\n")...)
		}
	}
	if len(out) > 6 {
		out = out[len(out)-6:]
	}
	return out
}

// ---- logo ----

// logoW × logoH is the logo bitmap: a hexagon with a stippled fill and a
// "K1" monogram. It is drawn with braille (2×4 pixels per cell) or, when
// braille may be missing from the font, with half blocks at half size.
const (
	logoW = 40
	logoH = 28
)

type bitmap struct {
	w, h   int
	on     []bool
	letter []bool // pixel belongs to the monogram (drawn in the accent colour)
}

func newBitmap(w, h int) *bitmap {
	return &bitmap{w: w, h: h, on: make([]bool, w*h), letter: make([]bool, w*h)}
}

func (b *bitmap) set(x, y int, letter bool) {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return
	}
	b.on[y*b.w+x] = true
	if letter {
		b.letter[y*b.w+x] = true
	}
}

func (b *bitmap) line(x0, y0, x1, y1 int, letter bool) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		b.set(x0, y0, letter)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var logoBitmap = buildLogo()

func buildLogo() *bitmap {
	b := newBitmap(logoW, logoH)
	cx, cy, r := 19.5, 13.5, 13.5
	var px, py [6]int
	for k := range 6 {
		a := math.Pi/2 + float64(k)*math.Pi/3
		px[k] = int(math.Round(cx + r*math.Cos(a)*1.15))
		py[k] = int(math.Round(cy - r*math.Sin(a)))
	}
	inside := func(x, y int) bool {
		// Point-in-convex-polygon test against the hexagon edges.
		for k := range 6 {
			x0, y0, x1, y1 := px[k], py[k], px[(k+1)%6], py[(k+1)%6]
			if (x1-x0)*(y-y0)-(y1-y0)*(x-x0) > 0 {
				return false
			}
		}
		return true
	}
	// Stippled fill: a sparse diagonal dot pattern inside the hexagon.
	for y := range logoH {
		for x := range logoW {
			if (x+2*y)%5 == 0 && y%2 == 0 && inside(x, y) {
				b.set(x, y, false)
			}
		}
	}
	for k := range 6 {
		b.line(px[k], py[k], px[(k+1)%6], py[(k+1)%6], false)
	}
	// Clear a band behind the monogram so it reads cleanly.
	for y := 6; y <= 21; y++ {
		for x := 11; x <= 29; x++ {
			if inside(x, y) && !isEdge(b, x, y, px, py) {
				b.on[y*b.w+x] = false
			}
		}
	}
	// "K"
	for _, dx := range []int{12, 13} {
		b.line(dx, 7, dx, 20, true)
	}
	b.line(14, 14, 21, 7, true)
	b.line(14, 13, 21, 20, true)
	b.line(15, 14, 22, 20, true)
	// "1"
	for _, dx := range []int{25, 26} {
		b.line(dx, 7, dx, 20, true)
	}
	b.line(22, 10, 25, 7, true)
	b.line(23, 20, 28, 20, true)
	return b
}

func isEdge(b *bitmap, x, y int, px, py [6]int) bool {
	for k := range 6 {
		x0, y0, x1, y1 := float64(px[k]), float64(py[k]), float64(px[(k+1)%6]), float64(py[(k+1)%6])
		// Distance from the point to the edge line.
		num := math.Abs((y1-y0)*float64(x) - (x1-x0)*float64(y) + x1*y0 - y1*x0)
		if num/math.Hypot(y1-y0, x1-x0) < 1 {
			return true
		}
	}
	return false
}

// logoLines renders the logo for the current font capabilities.
func (m *Model) logoLines() []string {
	b := logoBitmap
	style := func(s string, letter bool) string {
		if letter {
			return m.th.Accent.Render(s)
		}
		return m.th.Muted.Render(s)
	}
	if m.cfg.UI.Spinner == "line" {
		// Half blocks at half resolution (WGL4-safe).
		var out []string
		for y := 0; y+3 < b.h; y += 4 {
			var sb strings.Builder
			for x := 0; x+1 < b.w; x += 2 {
				// Each half block covers a 2×2 pixel area; any lit pixel lights it.
				any := func(x0, y0 int) (on, letter bool) {
					for dy := range 2 {
						for dx := range 2 {
							i := (y0+dy)*b.w + x0 + dx
							on = on || b.on[i]
							letter = letter || b.letter[i]
						}
					}
					return
				}
				top, l1 := any(x, y)
				bot, l2 := any(x, y+2)
				letter := l1 || l2
				switch {
				case top && bot:
					sb.WriteString(style("█", letter))
				case top:
					sb.WriteString(style("▀", letter))
				case bot:
					sb.WriteString(style("▄", letter))
				default:
					sb.WriteByte(' ')
				}
			}
			out = append(out, sb.String())
		}
		return out
	}
	// Braille: dot bits for (dx, dy) within a 2×4 cell.
	bits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	var out []string
	for cy := 0; cy < b.h; cy += 4 {
		var sb strings.Builder
		for cx := 0; cx < b.w; cx += 2 {
			var r rune
			letter := false
			for dy := range 4 {
				for dx := range 2 {
					x, y := cx+dx, cy+dy
					if x < b.w && y < b.h && b.on[y*b.w+x] {
						r |= bits[dy][dx]
						letter = letter || b.letter[y*b.w+x]
					}
				}
			}
			if r == 0 {
				sb.WriteByte(' ')
				continue
			}
			sb.WriteString(style(string(0x2800+r), letter))
		}
		out = append(out, sb.String())
	}
	return out
}
