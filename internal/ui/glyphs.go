package ui

import (
	"os"
	"runtime"
	"strings"

	"charm.land/bubbles/v2/spinner"
)

// Glyphs is the set of decorative symbols the UI draws. Fonts differ wildly in
// coverage (Consolas and the legacy Windows console fonts lack ❯, ◆, braille,
// rounded corners…) and a missing glyph renders as a boxed "?", so every
// non-ASCII symbol goes through this table.
type Glyphs struct {
	Prompt      string // input / user message prompt
	Think       string // reasoning line marker
	Interrupted string // under a cancelled/failed answer
	Error       string
	Warn        string
	Selected    string // completion menu cursor
	Thumb       string // scrollbar thumb
	More        string // "more content below"
	TL, TR      string // box corners
	BL, BR      string
	H, V        string // box edges
	Closed      string // collapsed section (click to open)
	Open        string // expanded section (click to close)
}

// UnicodeGlyphs is the rich set; needs a modern font (Cascadia, JetBrains
// Mono, SF Mono, Nerd Fonts…).
var UnicodeGlyphs = Glyphs{
	Prompt: "❯", Think: "◆", Interrupted: "⎿", Error: "✗", Warn: "⚠", Selected: "›",
	Thumb: "┃", More: "▼",
	TL: "╭", TR: "╮", BL: "╰", BR: "╯", H: "─", V: "│",
	Closed: "▸", Open: "▾",
}

// SafeGlyphs uses only characters from the WGL4 set, which practically every
// terminal font (including Consolas and Lucida Console) covers.
var SafeGlyphs = Glyphs{
	Prompt: ">", Think: "♦", Interrupted: "└", Error: "x", Warn: "!", Selected: "›",
	Thumb: "▐", More: "▼",
	TL: "┌", TR: "┐", BL: "└", BR: "┘", H: "─", V: "│",
	Closed: "►", Open: "▼",
}

// Spinners are the loading animations, by config name. Braille needs a font
// with the Braille Patterns block (Cascadia, Consolas on Windows 11, most
// Linux/macOS fonts); "line" is plain ASCII for anything else.
var Spinners = map[string]spinner.Spinner{
	"braille": {Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, FPS: spinner.MiniDot.FPS},
	"dots":    {Frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}, FPS: spinner.Dot.FPS},
	"pulse":   {Frames: []string{"⠁", "⠃", "⠇", "⡇", "⣇", "⣧", "⣷", "⣿", "⣶", "⣤", "⣀", "⡀"}, FPS: spinner.MiniDot.FPS},
	"line":    {Frames: []string{"|", "/", "-", "\\"}, FPS: spinner.Line.FPS},
}

// SpinnerNames lists Spinners in display order.
var SpinnerNames = []string{"braille", "dots", "pulse", "line"}

// ResolveSpinner returns the named spinner (braille if unknown).
func ResolveSpinner(name string) spinner.Spinner {
	if s, ok := Spinners[name]; ok {
		return s
	}
	return Spinners["braille"]
}

// ResolveGlyphs picks the glyph set for a config value: "unicode", "safe" or
// "auto". Auto chooses the safe set on Windows (font coverage there is the
// common failure) and on non-UTF-8 locales elsewhere.
func ResolveGlyphs(setting string) Glyphs {
	switch setting {
	case "unicode":
		return UnicodeGlyphs
	case "safe":
		return SafeGlyphs
	}
	if runtime.GOOS == "windows" {
		return SafeGlyphs
	}
	if !utf8Locale() {
		return SafeGlyphs
	}
	return UnicodeGlyphs
}

func utf8Locale() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	// Unset locale: macOS terminals are UTF-8; assume the same elsewhere.
	return true
}
