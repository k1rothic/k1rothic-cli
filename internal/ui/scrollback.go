package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/ui/markdown"
	"github.com/akutor/k1rothic-cli/internal/ui/theme"
)

// indent is the left (and right) inset of message content inside the
// scrollback; the user block uses it as horizontal padding so the prompt
// glyph and the assistant text line up.
const indent = 3

// timeFormat matches the "8:19 AM" style timestamps.
const timeFormat = "3:04 PM"

// Long user messages are folded to userPreviewLines when they wrap to more
// than userFoldLines lines.
const (
	userFoldLines    = 8
	userPreviewLines = 4
	liveThinkLines   = 3
)

type entryKind int

const (
	entryUser entryKind = iota
	entryAssistant
	entryNotice
	entryError
)

// entry is one item in the scrollback.
type entry struct {
	kind entryKind
	at   time.Time
	text string // user/notice/error text, or assistant markdown source

	reasoning  string
	thinkStart time.Time
	thinkEnd   time.Time // zero while still thinking
	streaming  bool
	cancelled  bool
	err        string

	open bool // thinking expanded (assistant) or full text shown (long user message)
}

func (e *entry) thoughtFor() time.Duration {
	if e.thinkStart.IsZero() {
		return 0
	}
	end := e.thinkEnd
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(e.thinkStart)
}

// foldable reports whether the entry has something to open or close.
func (e *entry) foldable(s *scrollback) bool {
	switch e.kind {
	case entryAssistant:
		return e.reasoning != ""
	case entryUser:
		return len(s.userLines(e)) > userFoldLines
	}
	return false
}

// hotspot marks a clickable line of the rendered scrollback.
type hotspot struct {
	line int
	e    *entry
}

// scrollback renders entries. Finished entries are rendered once and cached
// as a joined string; only the streaming tail is re-rendered on updates.
type scrollback struct {
	entries []*entry

	width int
	th    theme.Theme
	g     Glyphs
	md    *markdown.Renderer

	// Animation state for the streaming entry, set by the app on each tick.
	spinFrame string
	tick      int

	frozen      string // rendering of entries[:frozenCount]
	frozenLines int
	frozenHot   []hotspot
	frozenCount int

	hot []hotspot // hotspots of the last render (content line numbers)
}

func newScrollback(th theme.Theme, g Glyphs, width int) *scrollback {
	s := &scrollback{th: th, g: g, width: width, md: markdown.New(th.Dark, indent, mdWidth(width))}
	s.md.SetColors(markdown.Colors{H1: th.H1Hex, Heading: th.HeadingHex})
	return s
}

// mdWidth is the markdown wrap width: the full width minus the right inset
// (glamour applies the left margin itself, inside this width).
func mdWidth(w int) int { return max(w-indent, 10) }

func (s *scrollback) setWidth(w int) {
	if w == s.width {
		return
	}
	s.width = w
	s.md.SetWidth(mdWidth(w))
	s.invalidate()
}

func (s *scrollback) setTheme(th theme.Theme) {
	s.th = th
	s.md.SetDark(th.Dark)
	s.md.SetColors(markdown.Colors{H1: th.H1Hex, Heading: th.HeadingHex})
	s.invalidate()
}

// toggle opens or closes one entry.
func (s *scrollback) toggle(e *entry) {
	e.open = !e.open
	s.invalidate()
}

// toggleAll opens every foldable entry if any is closed, else closes all.
func (s *scrollback) toggleAll() {
	anyClosed := false
	for _, e := range s.entries {
		if e.foldable(s) && !e.open {
			anyClosed = true
		}
	}
	for _, e := range s.entries {
		if e.foldable(s) {
			e.open = anyClosed
		}
	}
	s.invalidate()
}

func (s *scrollback) invalidate() {
	s.frozen, s.frozenLines, s.frozenHot, s.frozenCount = "", 0, nil, 0
}

func (s *scrollback) clear() {
	s.entries = nil
	s.invalidate()
}

func (s *scrollback) add(e *entry) { s.entries = append(s.entries, e) }

func (s *scrollback) last() *entry {
	if len(s.entries) == 0 {
		return nil
	}
	return s.entries[len(s.entries)-1]
}

func lineCount(s string) int { return strings.Count(s, "\n") + 1 }

// joiner accumulates entries separated by a blank line, tracking hotspots.
type joiner struct {
	parts []string
	lines int
	hot   []hotspot
}

func (j *joiner) addRaw(text string, lines int, hot []hotspot) {
	off := 0
	if len(j.parts) > 0 {
		off = j.lines + 1 // the blank separator line
	}
	for _, h := range hot {
		j.hot = append(j.hot, hotspot{line: h.line + off, e: h.e})
	}
	j.parts = append(j.parts, text)
	j.lines = off + lines
}

func (j *joiner) String() string { return strings.Join(j.parts, "\n\n") }

// render returns the full scrollback content and updates s.hot.
func (s *scrollback) render() string {
	// Freeze every leading entry that is no longer streaming.
	n := s.frozenCount
	for n < len(s.entries) && !s.entries[n].streaming {
		n++
	}
	if n > s.frozenCount {
		j := joiner{}
		if s.frozenCount > 0 {
			j.addRaw(s.frozen, s.frozenLines, s.frozenHot)
		}
		for _, e := range s.entries[s.frozenCount:n] {
			text, hot := s.renderEntry(e)
			j.addRaw(text, lineCount(text), hot)
		}
		s.frozen, s.frozenLines, s.frozenHot, s.frozenCount = j.String(), j.lines, j.hot, n
	}
	if s.frozenCount == len(s.entries) {
		s.hot = s.frozenHot
		return s.frozen
	}
	j := joiner{}
	if s.frozenCount > 0 {
		j.addRaw(s.frozen, s.frozenLines, s.frozenHot)
	}
	for _, e := range s.entries[s.frozenCount:] {
		text, hot := s.renderEntry(e)
		j.addRaw(text, lineCount(text), hot)
	}
	s.hot = j.hot
	return j.String()
}

// hotspotAt returns the entry whose clickable line is line, if any.
func (s *scrollback) hotspotAt(line int) *entry {
	for _, h := range s.hot {
		if h.line == line {
			return h.e
		}
	}
	return nil
}

func (s *scrollback) renderEntry(e *entry) (string, []hotspot) {
	switch e.kind {
	case entryUser:
		return s.renderUser(e)
	case entryAssistant:
		return s.renderAssistant(e)
	case entryError:
		return s.indented(s.th.Error, s.g.Error+" "+e.text), nil
	default:
		return s.indented(s.th.Muted, e.text), nil
	}
}

// indented wraps text to the content width and insets it like assistant text.
func (s *scrollback) indented(st interface{ Render(...string) string }, text string) string {
	body := ansi.Wrap(text, max(s.width-2*indent, 10), "")
	lines := strings.Split(body, "\n")
	pad := strings.Repeat(" ", indent)
	for i, l := range lines {
		lines[i] = pad + st.Render(l)
	}
	return strings.Join(lines, "\n")
}

// rightAlign places tag at the right edge (minus the inset) of line, or
// returns ok=false when there is no room.
func (s *scrollback) rightAlign(line, tag string) (string, bool) {
	used := ansi.StringWidth(strings.TrimRight(ansi.Strip(line), "  "))
	gap := s.width - indent - used - ansi.StringWidth(tag)
	if gap < 2 {
		return line, false
	}
	return ansi.Truncate(line, used, "") + strings.Repeat(" ", gap) + tag, true
}

// userLines is the user's message wrapped to the block's text width.
func (s *scrollback) userLines(e *entry) []string {
	inner := max(s.width, 10) - 2*indent
	body := ansi.Wrap(strings.TrimRight(e.text, "\n"), max(inner-2, 4), "")
	return strings.Split(body, "\n")
}

func (s *scrollback) renderUser(e *entry) (string, []hotspot) {
	w := max(s.width, 10)
	inner := w - 2*indent
	bg := s.th.UserBlock
	ts := s.th.Muted.Inherit(bg).Render(e.at.Format(timeFormat))
	tsW := ansi.StringWidth(e.at.Format(timeFormat))

	src := s.userLines(e)
	foldable := len(src) > userFoldLines
	shown := src
	if foldable && !e.open {
		shown = src[:userPreviewLines]
	}
	lines := make([]string, len(shown))
	for i, l := range shown {
		prefix := "  "
		if i == 0 {
			prefix = s.g.Prompt + " "
		}
		lines[i] = s.th.Bold.Inherit(bg).Render(prefix + l)
	}
	if foldable {
		var hint string
		if e.open {
			hint = s.g.Open + " show less"
		} else {
			hint = s.g.Closed + " " + plural(len(src)-userPreviewLines, "more line") + " · click to expand"
		}
		lines = append(lines, s.th.Muted.Inherit(bg).Render("  "+hint))
	}
	if gap := inner - ansi.StringWidth(lines[0]) - tsW; gap >= 2 {
		lines[0] += bg.Render(strings.Repeat(" ", gap)) + ts
	} else {
		lines = append([]string{ts}, lines...)
	}
	out := bg.Width(w).Padding(1, indent).Render(strings.Join(lines, "\n"))
	var hot []hotspot
	if foldable {
		for i := range lineCount(out) {
			hot = append(hot, hotspot{line: i, e: e})
		}
	}
	return out, hot
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

func (s *scrollback) renderAssistant(e *entry) (string, []hotspot) {
	var parts []string
	var hot []hotspot
	if e.reasoning != "" {
		parts = append(parts, s.renderThinking(e))
		hot = append(hot, hotspot{line: 0, e: e})
	} else if e.streaming && e.text == "" && !e.cancelled {
		// Waiting for the first token: show the thinking animation.
		parts = append(parts, strings.Repeat(" ", indent)+s.th.Accent.Render(s.spinFrame)+" "+
			shimmer(s.th, "Thinking…", s.tick))
	}
	if e.text != "" {
		md := s.md.Render(e.text)
		lines := strings.SplitN(md, "\n", 2)
		if l, ok := s.rightAlign(lines[0], s.th.Muted.Render(e.at.Format(timeFormat))); ok {
			lines[0] = l
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	switch {
	case e.cancelled:
		parts = append(parts, s.indented(s.th.Warn, s.g.Interrupted+" Interrupted"))
	case e.err != "":
		parts = append(parts, s.indented(s.th.Error, s.g.Interrupted+" "+e.err))
	}
	return strings.Join(parts, "\n\n"), hot
}

func (s *scrollback) renderThinking(e *entry) string {
	thinking := e.thinkEnd.IsZero() && e.streaming
	pad := strings.Repeat(" ", indent)
	dur := FormatElapsed(e.thoughtFor())
	arrow := s.g.Closed
	if e.open {
		arrow = s.g.Open
	}
	var head string
	if thinking {
		head = pad + s.th.Accent.Render(s.spinFrame) + " " + shimmer(s.th, "Thinking…", s.tick) +
			s.th.Faint.Render(" "+dur+"  "+arrow)
	} else {
		head = pad + s.th.Faint.Render(arrow+" ") + s.th.Muted.Render("Thought") + s.th.Faint.Render(" for "+dur)
	}
	if !e.open && !thinking {
		return head
	}
	body := ansi.Wordwrap(strings.TrimSpace(e.reasoning), max(s.width-2*indent-2, 10), "")
	ls := strings.Split(body, "\n")
	if thinking && !e.open && len(ls) > liveThinkLines {
		// While thinking but collapsed, show only the last few lines live.
		ls = ls[len(ls)-liveThinkLines:]
	}
	for i, l := range ls {
		ls[i] = pad + "  " + s.th.Reasoning.Render(l)
	}
	return head + "\n" + strings.Join(ls, "\n")
}

// shimmer renders text muted with a bright highlight sweeping across it.
func shimmer(th theme.Theme, text string, tick int) string {
	rs := []rune(text)
	const band = 3
	span := len(rs) + band*2
	pos := tick%span - band
	var b strings.Builder
	for i, r := range rs {
		d := i - pos
		switch {
		case d >= 0 && d < band:
			b.WriteString(th.Shine.Render(string(r)))
		default:
			b.WriteString(th.Accent.Render(string(r)))
		}
	}
	return b.String()
}
