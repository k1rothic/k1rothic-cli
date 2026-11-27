package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// paint pads every row to the full window width and fills it with the
// theme's background colour. Painting cells ourselves works in every
// terminal, unlike setting the terminal's default background (OSC 11), which
// Windows Terminal and others ignore.
func (m *Model) paint(rows []string) string {
	seq := bgSeq(m.th.Background)
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		if w := ansi.StringWidth(r); w < m.width {
			r += strings.Repeat(" ", m.width-w)
		}
		if seq == "" {
			b.WriteString(r)
			continue
		}
		b.WriteString(seq)
		b.WriteString(withBg(r, seq))
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// bgSeq is the SGR sequence that sets c as the background ("" for nil).
func bgSeq(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

// withBg re-applies seq after every SGR sequence in s that clears the
// background (a full reset or "default background"), so styled spans keep
// the window colour behind them while spans with their own background (the
// user message block) keep theirs.
func withBg(s, seq string) string {
	var b strings.Builder
	b.Grow(len(s) + 64)
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] == ';' || s[j] == ':' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				b.WriteString(s[i : j+1])
				if clearsBg(s[i+2 : j]) {
					b.WriteString(seq)
				}
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// clearsBg reports whether SGR parameters reset the background: an empty
// list, a 0 parameter, or 49. Colour arguments of 38/48/58 are skipped so a
// "0" inside "38;5;0" does not count.
func clearsBg(params string) bool {
	if params == "" {
		return true
	}
	ps := strings.Split(strings.ReplaceAll(params, ":", ";"), ";")
	for i := 0; i < len(ps); i++ {
		n, err := strconv.Atoi(ps[i])
		if err != nil {
			if ps[i] == "" {
				return true // "\x1b[;1m" ≡ reset first
			}
			continue
		}
		switch n {
		case 0, 49:
			return true
		case 38, 48, 58:
			if i+1 < len(ps) {
				switch ps[i+1] {
				case "5":
					i += 2
				case "2":
					i += 4
				}
			}
		}
	}
	return false
}
