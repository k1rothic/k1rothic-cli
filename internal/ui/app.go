// Package ui is the Bubble Tea application for Mode A (native chat/agent).
package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/permissions"
	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
	"github.com/akutor/k1rothic-cli/internal/ui/theme"
)

const (
	maxInputLines  = 8
	maxPopupItems  = 8
	quitArmWindow  = 2 * time.Second
	narrowWidth    = 60
	minWidth       = 20
	maxEventsBatch = 256
)

// Options configures the app.
type Options struct {
	Config     config.Config
	ConfigPath string // where settings are saved; "" = don't persist

	// Registry builds adapters and holds API keys (OS keychain). Provider,
	// when set, overrides it for the initial provider (tests).
	Registry *registry.Registry
	Provider provider.Provider

	Cwd          string
	ContextLimit int // override the model's context window; 0 = derive
	Version      string
}

// Model is the root Bubble Tea model. It is used through a pointer.
type Model struct {
	opts Options
	cfg  config.Config
	th   theme.Theme
	g    Glyphs
	home string

	width, height int
	ready         bool

	prov       provider.Provider
	ctxLimit   int
	modelCache map[string][]provider.ModelInfo
	panel      panel

	// Terminal background handling: the original colour is queried at
	// start (and restored on exit); the theme background is only applied
	// after that answer, so the query never reports our own colour.
	origBg   color.Color
	bgReady  bool
	quitting bool

	lay    layout // row positions of the last View, for mouse clicks
	padTop int    // blank lines added above the scrollback to bottom-anchor it

	welcomeHot map[int]string // welcome screen: content line → command

	sb    *scrollback
	vp    viewport.Model
	input textarea.Model
	spin  spinner.Model

	approval    permissions.Mode
	allowAlways bool
	toolsOn     bool // agent mode (tools on) vs chat mode

	history []provider.Message

	streaming   bool
	streamID    int
	cancel      context.CancelFunc
	events      <-chan provider.Event
	streamStart time.Time
	turnUsage   provider.Usage // latest usage of the current/last turn
	ctxUsed     int            // context tokens occupied after the last turn

	showHelp    bool
	compSel     int
	quitArmedAt time.Time
	kbEnhanced  bool
}

// New builds the model.
func New(opts Options) *Model {
	m := &Model{
		opts:        opts,
		cfg:         opts.Config,
		th:          theme.New(opts.Config.UI.Color, opts.Config.UI.Theme != "light"),
		g:           ResolveGlyphs(opts.Config.UI.Glyphs),
		home:        homeDir(),
		approval:    opts.Config.ApprovalMode(),
		allowAlways: opts.Config.AllowAlwaysApprove(),
		toolsOn:     true,
		width:       80,
		height:      24,
		prov:        opts.Provider,
		modelCache:  map[string][]provider.ModelInfo{},
	}
	m.sb = newScrollback(m.th, m.g, m.width)
	if m.prov == nil && opts.Registry != nil {
		if p, err := m.openAdapter(m.cfg.Provider); err == nil {
			m.prov = p
		} else {
			m.notice(err.Error(), true)
		}
	}
	m.updateContextLimit()

	m.vp = viewport.New()
	m.vp.MouseWheelEnabled = true
	m.vp.MouseWheelDelta = 3

	m.input = textarea.New()
	m.input.Placeholder = "Ask anything · / for commands"
	m.input.ShowLineNumbers = false
	m.input.DynamicHeight = true
	m.input.MinHeight = 1
	m.input.MaxHeight = maxInputLines
	m.input.CharLimit = 0
	m.input.SetPromptFunc(2, func(pi textarea.PromptInfo) string {
		if pi.LineNumber == 0 {
			return m.g.Prompt + " "
		}
		return "  "
	})
	km := textarea.DefaultKeyMap()
	// Enter sends; these insert a newline. Shift+Enter needs a terminal that
	// reports keyboard enhancements (kitty protocol, Windows console);
	// Alt+Enter and Ctrl+J work everywhere.
	km.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
	m.input.KeyMap = km
	// Use the terminal's own cursor: a drawn (virtual) cursor plus the
	// terminal cursor showed up as two cursors on some Windows consoles.
	m.input.SetVirtualCursor(false)
	m.applyInputStyles()
	m.input.Focus()

	m.spin = spinner.New(spinner.WithSpinner(ResolveSpinner(m.cfg.UI.Spinner)))
	m.spin.Style = m.th.Accent
	if opts.Provider == nil {
		m.startupNotice()
	}
	return m
}

// setGlyphs swaps the symbol set live (settings).
func (m *Model) setGlyphs(g Glyphs) {
	m.g = g
	m.sb.g = g
	m.sb.invalidate()
	m.refresh()
}

// setSpinner swaps the loading animation live (settings).
func (m *Model) setSpinner(name string) {
	m.spin.Spinner = ResolveSpinner(name)
}

// applyTheme rebuilds the theme from the configured colour theme and the
// given background.
func (m *Model) applyTheme(dark bool) {
	m.setTheme(theme.New(m.cfg.UI.Color, dark))
}

func (m *Model) applyInputStyles() {
	st := textarea.DefaultStyles(m.th.Dark)
	for _, s := range []*textarea.StyleState{&st.Focused, &st.Blurred} {
		s.Base = lipgloss.NewStyle()
		s.CursorLine = lipgloss.NewStyle()
		s.Prompt = m.th.Bold
		s.Placeholder = m.th.Faint
		s.Text = lipgloss.NewStyle()
		s.EndOfBuffer = lipgloss.NewStyle()
	}
	st.Cursor.Blink = true
	m.input.SetStyles(st)
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	// Always ask: the answer picks dark/light for "auto" and is the colour
	// restored on exit. Terminals that never answer get a short timeout.
	return tea.Batch(tea.RequestBackgroundColor,
		tea.Tick(bgWait, func(time.Time) tea.Msg { return bgTimeoutMsg{} }))
}

// ---- messages ----

type streamMsg struct {
	id     int
	events []provider.Event
	closed bool
}

type disarmQuitMsg struct{}

// bgTimeoutMsg ends the wait for the terminal's background colour.
type bgTimeoutMsg struct{}

const bgWait = 400 * time.Millisecond

// quit restores the terminal background in a final frame, then exits.
func (m *Model) quit() tea.Cmd {
	m.cancelStream()
	m.quitting = true
	return tea.Quit
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, minWidth), max(msg.Height, 8)
		m.ready = true
		m.relayout(true)
		return m, nil

	case bgTimeoutMsg:
		m.bgReady = true
		return m, nil

	case tea.BackgroundColorMsg:
		if m.origBg == nil && !m.bgReady {
			m.origBg = msg.Color
		}
		m.bgReady = true
		if m.cfg.UI.Theme == "auto" {
			m.applyTheme(msg.IsDark())
		}
		return m, nil

	case tea.KeyboardEnhancementsMsg:
		m.kbEnhanced = msg.SupportsKeyDisambiguation()
		return m, nil

	case disarmQuitMsg:
		if !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) >= quitArmWindow {
			m.quitArmedAt = time.Time{}
		}
		return m, nil

	case streamMsg:
		return m, m.handleStream(msg)

	case modelsMsg:
		m.handleModels(msg)
		m.relayout(false)
		return m, nil

	case spinner.TickMsg:
		if !m.streaming {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.sb.spinFrame = m.spin.View()
		m.sb.tick++
		if e := m.sb.last(); e != nil && e.streaming && (e.text == "" || (e.reasoning != "" && e.thinkEnd.IsZero())) {
			m.refresh() // animate "Thinking…" and its timer
		}
		return m, cmd

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m, m.click(msg.X, msg.Y)
		}
		return m, nil

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.PasteMsg:
		if pr, ok := m.panel.(*prompt); ok {
			msg.Content = strings.TrimSpace(msg.Content)
			return m, pr.update(msg)
		}
		if m.panel != nil {
			return m, nil
		}
		// Normalize Windows/old-Mac line endings before they reach the buffer.
		msg.Content = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(msg.Content)
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.relayout(false)
		return m, cmd

	case tea.KeyPressMsg:
		if m.panel != nil {
			cur := m.panel
			cmd, closed := cur.key(m, msg)
			if closed && m.panel == cur {
				m.panel = nil
			}
			m.relayout(false)
			return m, cmd
		}
		if cmd, handled := m.handleKey(msg); handled {
			m.relayout(false)
			return m, cmd
		}
	}

	// Anything else (cursor blink, the clipboard text that Ctrl+V fetches
	// asynchronously, …) belongs to whichever text field is active: an open
	// prompt panel (API key, base URL) or the main input.
	if pr, ok := m.panel.(*prompt); ok {
		return m, pr.update(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.relayout(false)
	return m, tea.Batch(cmds...)
}

func (m *Model) setTheme(th theme.Theme) {
	m.th = th
	m.sb.setTheme(th)
	m.spin.Style = th.Accent
	m.applyInputStyles()
	m.refresh()
}

// handleKey processes app-level shortcuts. It returns handled=false for keys
// that should go to the text area.
func (m *Model) handleKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	matches := m.completions()

	switch k.String() {
	case "ctrl+c":
		switch {
		case m.streaming:
			m.cancelStream()
		case m.input.Value() != "":
			m.input.Reset()
		case !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) < quitArmWindow:
			return m.quit(), true
		default:
			m.quitArmedAt = time.Now()
			return tea.Tick(quitArmWindow, func(time.Time) tea.Msg { return disarmQuitMsg{} }), true
		}
		return nil, true

	case "esc":
		switch {
		case m.showHelp:
			m.showHelp = false
		case m.streaming:
			m.cancelStream()
		case len(matches) > 0:
			m.input.Reset()
		}
		return nil, true

	case "ctrl+d":
		if m.input.Value() == "" {
			return m.quit(), true
		}
		return nil, false

	case "shift+tab":
		m.approval = m.approval.Next(m.allowAlways)
		return nil, true

	case "ctrl+.", "f1":
		m.showHelp = !m.showHelp
		return nil, true

	case "f2":
		m.openSettings()
		return nil, true

	case "ctrl+o":
		m.sb.toggleAll()
		m.refresh()
		return nil, true

	case "pgup":
		m.vp.PageUp()
		return nil, true
	case "pgdown":
		m.vp.PageDown()
		return nil, true
	case "shift+up":
		m.vp.ScrollUp(1)
		return nil, true
	case "shift+down":
		m.vp.ScrollDown(1)
		return nil, true
	case "ctrl+home":
		m.vp.GotoTop()
		return nil, true
	case "ctrl+end":
		m.vp.GotoBottom()
		return nil, true
	}

	if len(matches) > 0 {
		switch k.String() {
		case "up":
			m.compSel = (m.compSel - 1 + len(matches)) % len(matches)
			return nil, true
		case "down":
			m.compSel = (m.compSel + 1) % len(matches)
			return nil, true
		case "tab":
			m.input.SetValue("/" + matches[m.compSel].Name + " ")
			m.compSel = 0
			return nil, true
		case "enter":
			name := matches[m.compSel].Name
			m.input.Reset()
			m.compSel = 0
			return m.runCommand(name, ""), true
		}
	}

	if k.String() == "enter" {
		return m.submit(), true
	}
	m.compSel = 0
	return nil, false
}

func (m *Model) completions() []command {
	q, ok := completionQuery(m.input.Value())
	if !ok {
		return nil
	}
	ms := matchCommands(q)
	if len(ms) > maxPopupItems {
		ms = ms[:maxPopupItems]
	}
	if m.compSel >= len(ms) {
		m.compSel = 0
	}
	return ms
}

func (m *Model) submit() tea.Cmd {
	raw := m.input.Value()
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	if name, args, ok := parseSlash(text); ok {
		m.input.Reset()
		return m.runCommand(name, args)
	}
	if m.streaming {
		m.notice("Still responding — press Esc to stop first.", false)
		return nil
	}
	m.input.Reset()
	m.sb.add(&entry{kind: entryUser, at: time.Now(), text: text})
	m.history = append(m.history, provider.Message{Role: provider.RoleUser, Text: text})
	return m.startStream()
}

func (m *Model) runCommand(name, args string) tea.Cmd {
	for _, c := range commands {
		if c.Name == name && c.Milestone != "" {
			m.notice(fmt.Sprintf("/%s is not implemented yet (planned for %s).", name, c.Milestone), false)
			return nil
		}
	}
	switch name {
	case "settings", "config":
		m.openSettings()
	case "theme":
		if args != "" {
			if !theme.Valid(args) {
				m.notice("Unknown theme "+args+". Themes: "+strings.Join(theme.Names, ", "), true)
				return nil
			}
			m.cfg.UI.Color = args
			m.applyTheme(m.th.Dark)
			m.save()
			return nil
		}
		return m.openThemePicker()
	case "provider":
		return m.providerCommand(args)
	case "endpoints", "endpoint":
		return m.openEndpointsManager()
	case "model":
		if args != "" {
			m.setModel(args)
			return nil
		}
		return m.openModelPicker()
	case "help":
		m.notice(helpText(m.kbEnhanced), false)
	case "clear":
		if m.streaming {
			m.cancelStream()
		}
		m.sb.clear()
		m.history = nil
		m.ctxUsed, m.turnUsage = 0, provider.Usage{}
		m.refresh()
	case "chat":
		m.toolsOn = false
		m.notice("Chat mode: tools are off.", false)
	case "agent":
		m.toolsOn = true
		m.notice("Agent mode: tools are on (the tools themselves arrive in M3).", false)
	case "quit", "exit":
		return m.quit()
	default:
		m.notice(fmt.Sprintf("Unknown command /%s — try /help.", name), true)
	}
	return nil
}

func (m *Model) notice(text string, isErr bool) {
	k := entryNotice
	if isErr {
		k = entryError
	}
	m.sb.add(&entry{kind: k, at: time.Now(), text: text})
	m.refresh()
	m.vp.GotoBottom()
}

// ---- streaming ----

func (m *Model) startStream() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.streamID++
	m.cancel = cancel
	m.streaming = true
	m.streamStart = time.Now()
	m.sb.spinFrame = m.spin.View()
	m.turnUsage = provider.Usage{}

	req := provider.Request{
		Model:    m.cfg.Model,
		Messages: append([]provider.Message(nil), m.history...),
		Effort:   provider.Effort(m.cfg.Effort),
	}
	if m.prov == nil {
		cancel()
		m.streaming = false
		m.notice("No provider configured — run /provider.", true)
		return nil
	}
	m.events = m.prov.Stream(ctx, req)
	m.sb.add(&entry{kind: entryAssistant, at: time.Now(), streaming: true})
	m.refresh()
	m.vp.GotoBottom()
	return tea.Batch(waitEvents(m.streamID, m.events), m.spin.Tick)
}

// waitEvents blocks for the next event, then drains whatever else is already
// buffered so bursts of tiny deltas cost one render instead of hundreds.
func waitEvents(id int, ch <-chan provider.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamMsg{id: id, closed: true}
		}
		evs := []provider.Event{ev}
		for len(evs) < maxEventsBatch {
			select {
			case ev, ok := <-ch:
				if !ok {
					return streamMsg{id: id, events: evs, closed: true}
				}
				evs = append(evs, ev)
			default:
				return streamMsg{id: id, events: evs}
			}
		}
		return streamMsg{id: id, events: evs}
	}
}

func (m *Model) handleStream(msg streamMsg) tea.Cmd {
	if msg.id != m.streamID {
		return nil // stale stream from a cancelled/cleared turn
	}
	e := m.sb.last()
	if e == nil || e.kind != entryAssistant {
		e = &entry{kind: entryAssistant, at: time.Now(), streaming: true}
		m.sb.add(e)
	}
	now := time.Now()
	for _, ev := range msg.events {
		switch ev.Kind {
		case provider.EventReasoningDelta:
			if e.thinkStart.IsZero() {
				e.thinkStart = now
			}
			e.reasoning += ev.Text
		case provider.EventTextDelta:
			if e.reasoning != "" && e.thinkEnd.IsZero() {
				e.thinkEnd = now
			}
			e.text += ev.Text
		case provider.EventUsage:
			if ev.Usage != nil {
				m.turnUsage = *ev.Usage
			}
		case provider.EventError:
			if errors.Is(ev.Err, context.Canceled) {
				e.cancelled = true
			} else if ev.Err != nil {
				e.err = ev.Err.Error()
			}
		}
	}
	if msg.closed {
		m.finishStream(e)
		m.refresh()
		return nil
	}
	m.refresh()
	return waitEvents(msg.id, m.events)
}

func (m *Model) finishStream(e *entry) {
	e.streaming = false
	if e.reasoning != "" && e.thinkEnd.IsZero() {
		e.thinkEnd = time.Now()
	}
	m.streaming = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.events = nil
	if t := m.turnUsage.Total(); t > 0 {
		m.ctxUsed = t
	}
	if e.text != "" {
		m.history = append(m.history, provider.Message{Role: provider.RoleAssistant, Text: e.text})
	}
}

func (m *Model) cancelStream() {
	if !m.streaming || m.cancel == nil {
		return
	}
	m.cancel()
	if e := m.sb.last(); e != nil && e.kind == entryAssistant {
		e.cancelled = true
	}
	m.refresh()
	// The stream goroutine closes its channel; handleStream finishes the turn.
}

// ---- layout & view ----

// padX is the outer horizontal padding around every row of the UI.
func (m *Model) padX() int {
	if m.width < narrowWidth {
		return 1
	}
	return 2
}

// contentWidth is the width available inside the outer padding.
func (m *Model) contentWidth() int { return max(m.width-2*m.padX(), 10) }

// compact drops the blank spacer rows on short terminals.
func (m *Model) compact() bool { return m.height < 18 }

func (m *Model) spacer() []string {
	if m.compact() {
		return nil
	}
	return []string{""}
}

// relayout recomputes widths and the viewport height from the variable-height
// widgets.
func (m *Model) relayout(force bool) {
	if !m.ready {
		return
	}
	cw := m.contentWidth()
	m.sb.setWidth(cw)
	m.input.SetWidth(max(cw-4, 4))

	fixed := 1 /*top*/ + 1 /*status*/ + 1 /*footer*/ + m.input.Height() + 2 /*box borders*/
	if !m.compact() {
		fixed += 2 /*outer padding: top and bottom of the screen*/ +
			3 /*spacers: under top bar, above input, above footer*/
	}
	fixed += m.popupHeight()
	h := max(m.height-fixed, 1)
	if force || h != m.vp.Height() || m.vp.Width() != cw {
		atBottom := m.vp.AtBottom()
		m.vp.SetWidth(cw)
		m.vp.SetHeight(h)
		m.vp.SetContent(m.content())
		if atBottom {
			m.vp.GotoBottom()
		}
	}
}

// popupHeight is the height of the open panel, help or completion popup.
func (m *Model) popupHeight() int {
	if m.panel != nil {
		rows, _ := m.panel.view(m)
		return len(rows)
	}
	if p := m.popupView(); p != "" {
		return lipgloss.Height(p)
	}
	return 0
}

// content is the scrollback anchored to the bottom of the viewport: short
// conversations sit right above the input instead of leaving a gap.
func (m *Model) content() string {
	if m.isNewChat() {
		return m.welcomeContent()
	}
	m.welcomeHot = nil
	lines := strings.Split(m.sb.render(), "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	m.padTop = 0
	if pad := m.vp.Height() - len(lines); pad > 0 {
		lines = append(make([]string, pad), lines...)
		m.padTop = pad
	}
	return strings.Join(lines, "\n")
}

// refresh re-renders scrollback content, following the bottom if the user
// had not scrolled up.
func (m *Model) refresh() {
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(m.content())
	if atBottom {
		m.vp.GotoBottom()
	}
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}
	// Render every fixed-height piece first, then give the scrollback exactly
	// the rows that are left. Sizing the viewport from what is actually drawn
	// (instead of a separate estimate) keeps the frame at the terminal height;
	// a frame one row too tall scrolls the screen and leaves ghost rows.
	top := append(append(append([]string{}, m.spacer()...), m.topBar()), m.spacer()...)
	var popup []string
	var pcur *tea.Cursor
	if m.panel != nil {
		popup, pcur = m.panel.view(m)
	} else if p := m.popupView(); p != "" {
		popup = strings.Split(p, "\n")
	}
	box := strings.Split(m.inputBox(), "\n")
	bottom := append(append(append([]string{}, m.spacer()...), m.footer()), m.spacer()...)
	aboveBox := m.spacer()
	fixed := len(top) + 1 /*status*/ + len(popup) + len(aboveBox) + len(box) + len(bottom)
	if h := max(m.height-fixed, 1); h != m.vp.Height() {
		atBottom := m.vp.AtBottom()
		m.vp.SetHeight(h)
		m.vp.SetContent(m.content())
		if atBottom {
			m.vp.GotoBottom()
		}
	}

	rows := top
	m.lay = layout{vpTop: len(rows)}
	rows = append(rows, m.scrollView()...)
	m.lay.statusY = len(rows)
	rows = append(rows, m.statusLine())
	var cursor *tea.Cursor
	m.lay.popupTop = len(rows)
	if pcur != nil {
		pcur.Y += len(rows)
		cursor = pcur
	}
	rows = append(rows, popup...)
	m.lay.popupRows = len(popup)
	rows = append(rows, aboveBox...)
	boxTop := len(rows)
	m.lay.boxTop = boxTop
	rows = append(rows, box...)
	m.lay.footerY = len(rows) + len(m.spacer())
	rows = append(rows, bottom...)
	// Hard guarantee for tiny terminals: never more rows than the screen.
	if len(rows) > m.height {
		rows = rows[len(rows)-m.height:]
	}

	v := tea.NewView(m.paint(rows))
	if m.panel == nil {
		if c := m.input.Cursor(); c != nil {
			c.X += m.padX() + 2 // "│ "
			c.Y += boxTop + 1
			cursor = c
		}
	}
	v.Cursor = cursor
	// The theme paints the whole window; on exit the terminal's own
	// background comes back (also for terminals that ignore the reset).
	switch {
	case m.quitting:
		v.BackgroundColor = m.origBg
	case m.bgReady:
		v.BackgroundColor = m.th.Background
	}
	v.AltScreen = true
	if m.cfg.UI.Mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.WindowTitle = "k1rothic · " + filepath.Base(m.opts.Cwd)
	return v
}

// row pads a content-width line with the outer padding on both sides.
func (m *Model) row(s string) string {
	p := strings.Repeat(" ", m.padX())
	s = ansi.Truncate(s, m.contentWidth(), "…")
	return p + s + strings.Repeat(" ", max(m.contentWidth()-ansi.StringWidth(s), 0)) + p
}

// spread lays out left and right parts across the content width.
func (m *Model) spread(left, right string) string {
	gap := m.contentWidth() - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return m.row(left)
	}
	return m.row(left + strings.Repeat(" ", gap) + right)
}

// scrollView renders the viewport with a thin scrollbar in the right padding.
func (m *Model) scrollView() []string {
	lines := strings.Split(m.vp.View(), "\n")
	total, vh := m.vp.TotalLineCount(), m.vp.Height()
	thumbTop, thumbH := -1, 0
	if total > vh && vh > 0 {
		thumbH = max(vh*vh/total, 1)
		thumbTop = int(float64(vh-thumbH)*m.vp.ScrollPercent() + 0.5)
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		r := m.row(l)
		if thumbTop >= 0 && i >= thumbTop && i < thumbTop+thumbH {
			// Replace the last padding cell with the thumb.
			r = ansi.Truncate(r, m.width-1, "") + m.th.Faint.Render(m.g.Thumb)
		}
		out[i] = r
	}
	return out
}

func (m *Model) topBar() string {
	right := m.contextMeter()
	left := ShortenPath(m.opts.Cwd, m.home, max(m.contentWidth()-ansi.StringWidth(right)-3, 4))
	return m.spread(m.th.Muted.Render(left), right)
}

func (m *Model) statusLine() string {
	if m.streaming {
		e := m.sb.last()
		verb := "Responding…"
		if e != nil && e.text == "" {
			verb = "Thinking…"
		}
		phase := m.streamStart
		if e != nil && !e.thinkEnd.IsZero() {
			phase = e.thinkEnd
		}
		left := " " + m.spin.View() + " " + shimmer(m.th, verb, m.sb.tick) + " " +
			m.th.Faint.Render(FormatElapsed(time.Since(phase)))
		right := FormatElapsed(time.Since(m.streamStart))
		if out := m.turnUsage.OutputTokens; out > 0 {
			right += " ↓" + FormatTokensFine(out)
		}
		return m.spread(left, m.th.Muted.Render(right)+" "+m.th.Faint.Render("[stop]"))
	}
	if !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) < quitArmWindow {
		return m.row(" " + m.th.Warn.Render("Press Ctrl+C again to exit"))
	}
	if !m.vp.AtBottom() {
		// Scrolled up: show where the rest of the conversation is.
		return m.row(strings.Repeat(" ", m.contentWidth()/2) + m.th.Faint.Render(m.g.More))
	}
	return ""
}

func (m *Model) popupView() string {
	if m.showHelp {
		return m.helpPanel()
	}
	matches := m.completions()
	if len(matches) == 0 {
		return ""
	}
	nameW := 0
	for _, c := range matches {
		nameW = max(nameW, len(c.Name)+1)
	}
	lines := make([]string, len(matches))
	for i, c := range matches {
		name := fmt.Sprintf("%-*s", nameW, "/"+c.Name)
		desc := c.Desc
		if c.Milestone != "" {
			desc += " (" + c.Milestone + ")"
		}
		var line string
		if i == m.compSel {
			line = m.th.Selected.Render(m.g.Selected+" "+name) + "  " + m.th.Text.Render(desc)
		} else {
			line = m.th.Muted.Render("  "+name) + "  " + m.th.Faint.Render(desc)
		}
		lines[i] = m.row(" " + line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) helpPanel() string {
	rows := [][2]string{
		{"Enter", "send"},
		{"Shift+Enter · Alt+Enter · Ctrl+J", "newline"},
		{"Shift+Tab", "cycle approval mode"},
		{"Ctrl+C · Esc", "stop response"},
		{"Ctrl+C ×2 · Ctrl+D", "exit"},
		{"Ctrl+O · click", "open/close thinking and long prompts"},
		{"Click", "pick menu items, move the cursor, footer shortcuts, [stop]"},
		{"PgUp/PgDn · Shift+↑/↓", "scroll"},
		{"/", "commands"},
		{"F2 · /settings", "settings"},
		{"/theme", "color theme (live preview)"},
		{"Ctrl+. · F1", "toggle this help"},
	}
	kw := 0
	for _, r := range rows {
		kw = max(kw, ansi.StringWidth(r[0]))
	}
	lines := []string{m.row(" " + m.th.Accent.Render("Shortcuts"))}
	for _, r := range rows {
		lines = append(lines, m.row("   "+m.th.Bold.Render(fmt.Sprintf("%-*s", kw, r[0]))+"  "+m.th.Muted.Render(r[1])))
	}
	if !m.kbEnhanced {
		lines = append(lines, m.row("   "+m.th.Faint.Render("This terminal may not report Shift+Enter or Ctrl+. — use Alt+Enter / F1.")))
	}
	return strings.Join(lines, "\n")
}

// inputBox draws the rounded box around the text area with the
// "model (effort) · approval mode" label embedded in the bottom border.
func (m *Model) inputBox() string {
	w := m.contentWidth()
	inner := w - 4
	bs := m.th.BorderColor
	if m.streaming {
		bs = m.th.BorderColorBlur
	}
	rows := []string{m.row(bs.Render(m.g.TL + strings.Repeat(m.g.H, w-2) + m.g.TR))}
	for _, line := range strings.Split(m.input.View(), "\n") {
		line = ansi.Truncate(line, inner, "")
		pad := max(inner-ansi.StringWidth(line), 0)
		rows = append(rows, m.row(bs.Render(m.g.V)+" "+line+strings.Repeat(" ", pad)+" "+bs.Render(m.g.V)))
	}

	const tail = 2 // dashes between the label and the corner
	label := m.inputLabel()
	if maxL := w - 4 - tail; ansi.StringWidth(label) > maxL {
		label = ansi.Truncate(label, max(maxL, 0), "…")
	}
	ls := m.th.Muted
	if m.approval == permissions.AlwaysApprove {
		ls = m.th.Warn
	}
	lead := w - 2 - tail - 2 - ansi.StringWidth(label)
	if label == "" || lead < 1 {
		rows = append(rows, m.row(bs.Render(m.g.BL+strings.Repeat(m.g.H, w-2)+m.g.BR)))
	} else {
		rows = append(rows, m.row(bs.Render(m.g.BL+strings.Repeat(m.g.H, lead))+" "+ls.Render(label)+" "+bs.Render(strings.Repeat(m.g.H, tail)+m.g.BR)))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) inputLabel() string {
	model := m.cfg.Model
	mode := m.approval.String()
	if m.width < narrowWidth {
		return model + " · " + mode
	}
	return fmt.Sprintf("%s (%s) · %s", model, m.cfg.Effort, mode)
}

func (m *Model) footer() string {
	hint := func(k, d string) string { return m.th.Bold.Render(k) + m.th.Faint.Render(":"+d) }
	sep := m.th.Faint.Render(footerSep)
	var parts []string
	for _, h := range m.footerHints() {
		parts = append(parts, hint(h.key, h.desc))
	}
	left := strings.Join(parts, sep)
	mode := "agent"
	if !m.toolsOn {
		mode = "chat"
	}
	right := m.th.Faint.Render(mode)
	if m.approval == permissions.AlwaysApprove {
		// Persistent warning while tools run without asking.
		right = m.th.Warn.Render(m.g.Warn+" always-approve") + m.th.Faint.Render(" · "+mode)
	}
	return m.spread(left, right)
}

func helpText(kbEnhanced bool) string {
	var b strings.Builder
	b.WriteString("Commands:\n")
	for _, c := range commands {
		suffix := ""
		if c.Milestone != "" {
			suffix = " (" + c.Milestone + ")"
		}
		fmt.Fprintf(&b, "  /%-9s %s%s\n", c.Name, c.Desc, suffix)
	}
	b.WriteString("\nShortcuts: enter send · shift+enter/alt+enter/ctrl+j newline · shift+tab cycle approval · ctrl+c/esc stop · ctrl+o reasoning · ctrl+./f1 help")
	if !kbEnhanced {
		b.WriteString("\nNote: this terminal may not report shift+enter or ctrl+. — alt+enter and f1 always work.")
	}
	return b.String()
}
