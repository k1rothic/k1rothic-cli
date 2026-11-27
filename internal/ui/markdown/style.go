package markdown

import (
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// Style returns the app's markdown style: glamour's base palette with plain
// coloured headings (no "##" prefixes or H1 background), a short faint rule,
// and a configurable left margin.
func Style(dark bool, margin uint) ansi.StyleConfig { return StyleWith(dark, margin, Colors{}) }

// Colors overrides the heading colours (hex); empty fields keep the defaults.
type Colors struct{ H1, Heading string }

// StyleWith is Style with theme heading colours.
func StyleWith(dark bool, margin uint, c Colors) ansi.StyleConfig {
	base := styles.DarkStyleConfig
	h1, heading, text, rule, item := "#5FD7A7", "#7AA2F7", "#D4D4D4", "#4B5263", "#8B949E"
	if !dark {
		base = styles.LightStyleConfig
		h1, heading, text, rule, item = "#1A7F5A", "#0B63D6", "#24292F", "#C2C8CF", "#6E7781"
	}
	if c.H1 != "" {
		h1 = c.H1
	}
	if c.Heading != "" {
		heading = c.Heading
	}
	cfg := base // shallow copy; only top-level fields are replaced below

	cfg.Document = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Color: &text},
		Margin:         &margin,
	}
	cfg.Heading = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
		BlockSuffix: "\n", Color: &heading, Bold: ptr(true),
	}}
	cfg.H1 = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &h1, Bold: ptr(true)}}
	cfg.H2 = ansi.StyleBlock{}
	cfg.H3 = ansi.StyleBlock{}
	cfg.H4 = ansi.StyleBlock{}
	cfg.H5 = ansi.StyleBlock{}
	cfg.H6 = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: ptr(false)}}
	cfg.HorizontalRule = ansi.StylePrimitive{Color: &rule, Format: "\n───\n"}
	cfg.Item = ansi.StylePrimitive{BlockPrefix: "• ", Color: &item}
	cfg.CodeBlock.Margin = ptr[uint](0)
	return cfg
}

func ptr[T any](v T) *T { return &v }
