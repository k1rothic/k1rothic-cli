// Package theme defines the colour themes and Lip Gloss styles.
//
// Colours are specified in truecolor; Bubble Tea v2 downsamples them to the
// terminal's detected profile (256, 16 colours or none, honouring NO_COLOR),
// so the UI degrades without extra code here.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is the resolved set of styles for one colour theme and background.
type Theme struct {
	Name string
	Dark bool

	Text      lipgloss.Style
	Muted     lipgloss.Style
	Faint     lipgloss.Style
	Accent    lipgloss.Style
	Warn      lipgloss.Style
	Error     lipgloss.Style
	Success   lipgloss.Style
	UserBlock lipgloss.Style
	Reasoning lipgloss.Style
	Selected  lipgloss.Style
	Shine     lipgloss.Style // highlight that sweeps over "Thinking…"

	Bold            lipgloss.Style // emphasised primary text (prompts, keys)
	BorderColor     lipgloss.Style // foreground = border colour (focused)
	BorderColorBlur lipgloss.Style

	// Background is painted behind the whole window: the terminal's default
	// background is set while the app runs and restored on exit.
	Background color.Color

	// Markdown heading colours (hex) for this theme and background.
	H1Hex, HeadingHex string
}

// pair is a colour for dark and light backgrounds.
type pair struct{ dark, light string }

func (p pair) pick(dark bool) string {
	if dark {
		return p.dark
	}
	return p.light
}

// palette holds the colours that differ between themes. Text, warnings and
// errors stay the same everywhere for readability.
type palette struct {
	label      string
	accent     pair
	h1         pair
	userBg     pair
	border     pair
	borderBlur pair
	muted      pair
	faint      pair
	text       pair
	bg         pair // window background painted behind the whole UI
	darkOnly   bool // ignore a light terminal background
}

// Default is the theme used when none is configured.
const Default = "carbon"

var palettes = map[string]palette{
	"carbon": {label: "Carbon (near-black window, grey blocks)", darkOnly: true, bg: pair{"#151515", "#151515"},
		accent: pair{"#7AA2F7", "#7AA2F7"}, h1: pair{"#5FD7A7", "#5FD7A7"},
		userBg: pair{"#2B2B2B", "#2B2B2B"}, border: pair{"#3D3D3D", "#3D3D3D"}, borderBlur: pair{"#2C2C2C", "#2C2C2C"},
		muted: pair{"#8A8A8A", "#8A8A8A"}, faint: pair{"#5E5E5E", "#5E5E5E"}, text: pair{"#D4D4D4", "#D4D4D4"}},
	"graphite": {label: "Graphite (neutral grey, blue accent)",
		bg:     pair{"#1E1F22", "#F6F7F9"},
		accent: pair{"#7AA2F7", "#0B63D6"}, h1: pair{"#5FD7A7", "#1A7F5A"},
		userBg: pair{"#2A2A2A", "#EEF1F5"}, border: pair{"#4A4A4A", "#AEB4BB"}, borderBlur: pair{"#333333", "#D0D7DE"},
		muted: pair{"#8B949E", "#6E7781"}, faint: pair{"#5A6069", "#A0A7B0"}},
	"ocean": {label: "Ocean (deep blue, cyan)",
		bg:     pair{"#0E1621", "#F2F7FC"},
		accent: pair{"#5EC4FF", "#0969DA"}, h1: pair{"#7FDBCA", "#0E7C86"},
		userBg: pair{"#162332", "#E6F0FA"}, border: pair{"#2D4A66", "#9DB8D3"}, borderBlur: pair{"#1F3347", "#C8D9EA"},
		muted: pair{"#8AA4BF", "#5B738C"}, faint: pair{"#4D6680", "#9FB3C8"}},
	"forest": {label: "Forest (green, warm gold)",
		bg:     pair{"#111813", "#F3F8F4"},
		accent: pair{"#8BD49C", "#1F7A3A"}, h1: pair{"#E6C07B", "#9A6700"},
		userBg: pair{"#1C2A21", "#EAF3EC"}, border: pair{"#3A5442", "#A9C4B0"}, borderBlur: pair{"#26382C", "#D2E2D6"},
		muted: pair{"#93A89A", "#61756A"}, faint: pair{"#56685C", "#A3B5A9"}},
	"amber": {label: "Amber (warm orange)",
		bg:     pair{"#18140E", "#FCF7EF"},
		accent: pair{"#F5B556", "#B35900"}, h1: pair{"#FF9B73", "#C2410C"},
		userBg: pair{"#2B2418", "#FBF1E3"}, border: pair{"#5A4A30", "#CDB894"}, borderBlur: pair{"#3A3122", "#E6D8C0"},
		muted: pair{"#B0A189", "#7A6A52"}, faint: pair{"#6E624F", "#B5A68E"}},
	"rose": {label: "Rose (pink, lavender)",
		bg:     pair{"#1A1317", "#FCF3F7"},
		accent: pair{"#F591B2", "#C2255C"}, h1: pair{"#C8A6F7", "#7048C8"},
		userBg: pair{"#2A1F26", "#FAEEF3"}, border: pair{"#5A3C4C", "#D3AFC0"}, borderBlur: pair{"#3B2A33", "#EAD5DF"},
		muted: pair{"#B3979F", "#7F6470"}, faint: pair{"#6E5560", "#B89FAA"}},
	"violet": {label: "Violet (purple, sky)",
		bg:     pair{"#15121E", "#F6F4FD"},
		accent: pair{"#B49CFF", "#6741D9"}, h1: pair{"#7DD3FC", "#0369A1"},
		userBg: pair{"#221E33", "#F0EDFB"}, border: pair{"#463E66", "#B9B0D9"}, borderBlur: pair{"#2E2945", "#DAD4EE"},
		muted: pair{"#A09AB8", "#6A6485"}, faint: pair{"#5F5978", "#A9A3C2"}},
	"mono": {label: "Mono (greyscale, no colour accents)",
		bg:     pair{"#0F0F0F", "#FFFFFF"},
		accent: pair{"#E6E6E6", "#1F2328"}, h1: pair{"#FFFFFF", "#000000"},
		userBg: pair{"#262626", "#EFEFEF"}, border: pair{"#4D4D4D", "#B0B0B0"}, borderBlur: pair{"#333333", "#D6D6D6"},
		muted: pair{"#9A9A9A", "#6B6B6B"}, faint: pair{"#5C5C5C", "#A8A8A8"}},
}

// Names lists the themes in display order.
var Names = []string{"carbon", "graphite", "ocean", "forest", "amber", "rose", "violet", "mono"}

// Label is the human description of a theme.
func Label(name string) string {
	if p, ok := palettes[name]; ok {
		return p.label
	}
	return name
}

// Valid reports whether name is a known theme.
func Valid(name string) bool { _, ok := palettes[name]; return ok }

// New builds theme name (Default if unknown) for a dark or light background.
func New(name string, dark bool) Theme {
	p, ok := palettes[name]
	if !ok {
		name, p = Default, palettes[Default]
	}
	if p.darkOnly {
		dark = true
	}
	c := func(x pair) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(x.pick(dark))) }
	text := pair{"#E6E6E6", "#1F2328"}
	if p.text.dark != "" {
		text = p.text
	}
	var bg color.Color
	if c := p.bg.pick(dark); c != "" {
		bg = lipgloss.Color(c)
	}
	return Theme{
		Background:      bg,
		Name:            name,
		Dark:            dark,
		Text:            c(text),
		Muted:           c(p.muted),
		Faint:           c(p.faint),
		Accent:          c(p.accent),
		Warn:            c(pair{"#E0AF68", "#9A6700"}),
		Error:           c(pair{"#F7768E", "#CF222E"}),
		Success:         c(pair{"#9ECE6A", "#1A7F37"}),
		UserBlock:       lipgloss.NewStyle().Background(lipgloss.Color(p.userBg.pick(dark))).Foreground(lipgloss.Color(text.pick(dark))),
		Reasoning:       c(p.muted).Italic(true),
		Selected:        c(p.accent).Bold(true),
		Shine:           c(text).Bold(true),
		BorderColor:     c(p.border),
		BorderColorBlur: c(p.borderBlur),
		Bold:            c(text).Bold(true),
		H1Hex:           p.h1.pick(dark),
		HeadingHex:      p.accent.pick(dark),
	}
}
