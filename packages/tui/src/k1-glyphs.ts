// k1rothic: symbols that many terminal fonts lack (Consolas, the legacy
// Windows console fonts…) render as a boxed "?". In "safe" mode every such
// symbol is swapped for one from the WGL4 set that practically every font has.
//
// K1ROTHIC_GLYPHS=safe|unicode overrides the default (safe on Windows).
const setting = process.env.K1ROTHIC_GLYPHS?.toLowerCase()
export const safeGlyphs = setting === "safe" || (setting !== "unicode" && process.platform === "win32")
// A terminal cannot report whether its font has a glyph, so this is decided from the host. macOS and
// Linux terminals fall back to a font with braille. On Windows only the legacy console lacks that
// fallback (it draws a boxed glyph); it sets none of these variables, so there the spinner becomes a
// rotating ASCII bar. K1ROTHIC_GLYPHS=unicode|safe overrides the detection.
const env = process.env
const modernWindowsTerminal = Boolean(
  env.WT_SESSION || env.TERM_PROGRAM || env.WEZTERM_EXECUTABLE || env.ALACRITTY_LOG || env.MSYSTEM || (env.TERM && env.TERM !== "dumb"),
)
const asciiSpinner = setting === "safe" || (setting !== "unicode" && process.platform === "win32" && !modernWindowsTerminal)

// Braille spinner frames become a rotating ASCII bar, frame for frame.
const braille = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⣾⣽⣻⢿⡿⣟⣯⣷⠁⠃⠇⡇⣇⣧⣷⣿⣶⣤⣀⡀"
const bar = ["|", "/", "-", "\\"]

const table: Record<string, string> = {
  "✓": "√", "✔": "√", "✗": "x", "✘": "x", "✕": "x", "✖": "x",
  "✱": "*", "⚙": "*", "◈": "♦", "◆": "♦", "◇": "◊", "⬥": "♦", "⬩": "·", "⬪": "·", "⬝": "·", "⬖": "♦",
  "❯": ">", "↳": "└", "⇆": "↔", "↵": "<", "↻": "o", "⟳": "o", "⎇": "Y", "⌕": "?",
  "△": "▲", "▶": "►", "▸": "►", "▾": "▼", "⋯": "…", "⚠": "!",
  "⊙": "○", "◎": "○", "◉": "●", "⦿": "●", "◍": "○", "◔": "○",
  "◜": "|", "◝": "/", "◞": "-", "◟": "\\", "◴": "|", "◷": "/", "◶": "-", "◵": "\\",
  "┃": "│", "╹": "│", "╭": "┌", "╮": "┐", "╰": "└", "╯": "┘",
}
if (asciiSpinner) for (let i = 0; i < braille.length; i++) table[braille[i]!] = bar[i % bar.length]!

const pattern = new RegExp(`[${Object.keys(table).join("").replace(/[\\\]^-]/g, "\\$&")}]`, "gu")

/** The text with font-safe symbols in safe mode; unchanged otherwise. */
export function sym(text: string): string
export function sym(text: string | undefined): string | undefined
export function sym(text: string | undefined) {
  if (!safeGlyphs || typeof text !== "string") return text
  return text.replace(pattern, (ch) => table[ch] ?? ch)
}

/** For OpenTUI box borders: rounded corners only when the font has them. */
export const roundedBorder = (): "rounded" | "single" => (safeGlyphs ? "single" : "rounded")
