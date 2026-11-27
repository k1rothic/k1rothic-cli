// Package markdown renders streamed markdown incrementally.
//
// Re-rendering the whole message on every delta is O(n²) over a long answer.
// Instead the source is split into top-level blocks; every block except the
// last is complete (text after it already arrived) and its rendering is
// cached. Only the trailing, still-growing block is re-rendered per delta.
package markdown

import (
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

const maxCache = 1024

// Renderer renders markdown at a fixed width and style. Not safe for
// concurrent use; the UI owns one.
type Renderer struct {
	width  int
	dark   bool
	margin uint
	colors Colors
	tr     *glamour.TermRenderer
	cache  map[string]string
}

// New creates a renderer for a dark or light background. margin is the left
// indent (in cells) applied to every rendered line; width includes it.
func New(dark bool, margin uint, width int) *Renderer {
	r := &Renderer{dark: dark, margin: margin}
	r.SetWidth(width)
	return r
}

// SetWidth changes the wrap width, invalidating the cache when it changes.
func (r *Renderer) SetWidth(width int) {
	width = max(width, 10)
	if width == r.width && r.tr != nil {
		return
	}
	r.width = width
	r.reset()
}

// SetColors changes the heading colours (empty fields keep the defaults).
func (r *Renderer) SetColors(c Colors) {
	if c == r.colors && r.tr != nil {
		return
	}
	r.colors = c
	r.reset()
}

// SetDark switches between the dark and light palettes.
func (r *Renderer) SetDark(dark bool) {
	if dark == r.dark && r.tr != nil {
		return
	}
	r.dark = dark
	r.reset()
}

func (r *Renderer) reset() {
	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(StyleWith(r.dark, r.margin, r.colors)),
		glamour.WithWordWrap(r.width),
	)
	if err != nil {
		tr = nil
	}
	r.tr = tr
	r.cache = make(map[string]string)
}

// Render renders src. Completed blocks come from cache.
func (r *Renderer) Render(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	blocks := SplitBlocks(src)
	out := make([]string, 0, len(blocks))
	for i, b := range blocks {
		last := i == len(blocks)-1
		if !last {
			out = append(out, r.renderCached(b))
			continue
		}
		out = append(out, r.renderBlock(closeOpenFence(b)))
	}
	return strings.Join(nonEmpty(out), "\n\n")
}

func (r *Renderer) renderCached(b string) string {
	if s, ok := r.cache[b]; ok {
		return s
	}
	s := r.renderBlock(b)
	if len(r.cache) >= maxCache {
		r.cache = make(map[string]string)
	}
	r.cache[b] = s
	return s
}

func (r *Renderer) renderBlock(b string) string {
	if strings.TrimSpace(b) == "" {
		return ""
	}
	if r.tr == nil {
		return ansi.Wordwrap(b, r.width, "")
	}
	s, err := r.tr.Render(b)
	if err != nil {
		return ansi.Wordwrap(b, r.width, "")
	}
	return trimBlankLines(s)
}

var (
	fenceRe    = regexp.MustCompile("^ {0,3}(```+|~~~+)")
	listItemRe = regexp.MustCompile(`^ {0,3}([-*+]|\d{1,9}[.)])(\s|$)`)
	tableRe    = regexp.MustCompile(`^ {0,3}\|`)
)

// SplitBlocks splits markdown into top-level blocks at blank lines that are
// outside code fences and not followed by a continuation (indented line, list
// item or table row), so lists and code stay in one block.
func SplitBlocks(src string) []string {
	lines := strings.Split(src, "\n")
	var blocks []string
	var cur []string
	fence := "" // active fence marker, e.g. "```"

	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for i, line := range lines {
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			marker := m[1]
			switch {
			case fence == "":
				fence = marker
			case marker[0] == fence[0] && len(marker) >= len(fence) && strings.TrimSpace(line[strings.Index(line, marker)+len(marker):]) == "":
				fence = ""
			}
			cur = append(cur, line)
			continue
		}
		if fence == "" && strings.TrimSpace(line) == "" {
			next, ok := nextNonBlank(lines, i+1)
			if ok && !isContinuation(next, cur) {
				flush()
				continue
			}
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}

func nextNonBlank(lines []string, from int) (string, bool) {
	for j := from; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) != "" {
			return lines[j], true
		}
	}
	return "", false
}

// isContinuation reports whether line (after a blank line) still belongs to
// the current block: indented content, a table row, or another item of a list
// the block already is.
func isContinuation(line string, cur []string) bool {
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") || tableRe.MatchString(line) {
		return true
	}
	return listItemRe.MatchString(line) && len(cur) > 0 && listItemRe.MatchString(cur[0])
}

// closeOpenFence appends a closing fence when a code block is still streaming
// so that it is highlighted as code instead of flashing as prose.
func closeOpenFence(b string) string {
	fence := ""
	for _, line := range strings.Split(b, "\n") {
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			if fence == "" {
				fence = m[1]
			} else if m[1][0] == fence[0] && len(m[1]) >= len(fence) {
				fence = ""
			}
		}
	}
	if fence == "" {
		return b
	}
	return b + "\n" + fence
}

func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(ansi.Strip(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(ansi.Strip(lines[end-1])) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

func nonEmpty(xs []string) []string {
	out := xs[:0]
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
