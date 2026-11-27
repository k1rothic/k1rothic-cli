// Generates the k1rothic colour themes (v1 theme JSON) into
// packages/tui/src/theme/assets/. Re-run after editing the palettes:
//
//   bun script/k1rothic/gen-themes.ts
import path from "path"

type Pair = [dark: string, light: string]
type Palette = {
  bg: Pair
  panel: Pair // user message / raised blocks
  element: Pair
  border: Pair
  borderSubtle: Pair
  accent: Pair
  secondary: Pair // H1 headings, functions
  text: Pair
  muted: Pair
}

const light = {
  bg: "#F7F7F8",
  panel: "#ECEDEF",
  element: "#E2E4E8",
  border: "#C9CDD3",
  borderSubtle: "#DFE2E6",
  text: "#1F2328",
  muted: "#6E7781",
}

const palettes: Record<string, Palette> = {
  carbon: {
    bg: ["#151515", light.bg], panel: ["#2B2B2B", light.panel], element: ["#222222", light.element],
    border: ["#3D3D3D", light.border], borderSubtle: ["#262626", light.borderSubtle],
    accent: ["#7AA2F7", "#0B63D6"], secondary: ["#5FD7A7", "#1A7F5A"],
    text: ["#D4D4D4", light.text], muted: ["#8A8A8A", light.muted],
  },
  graphite: {
    bg: ["#1E1F22", "#F6F7F9"], panel: ["#2C2E33", "#EEF1F5"], element: ["#26282C", "#E6E9EE"],
    border: ["#4A4A4A", "#AEB4BB"], borderSubtle: ["#333333", "#D0D7DE"],
    accent: ["#7AA2F7", "#0B63D6"], secondary: ["#5FD7A7", "#1A7F5A"],
    text: ["#E6E6E6", light.text], muted: ["#8B949E", "#6E7781"],
  },
  ocean: {
    bg: ["#0E1621", "#F2F7FC"], panel: ["#162332", "#E6F0FA"], element: ["#122030", "#DCE8F5"],
    border: ["#2D4A66", "#9DB8D3"], borderSubtle: ["#1F3347", "#C8D9EA"],
    accent: ["#5EC4FF", "#0969DA"], secondary: ["#7FDBCA", "#0E7C86"],
    text: ["#DCE6F0", light.text], muted: ["#8AA4BF", "#5B738C"],
  },
  forest: {
    bg: ["#111813", "#F3F8F4"], panel: ["#1C2A21", "#EAF3EC"], element: ["#17221B", "#DFEBE2"],
    border: ["#3A5442", "#A9C4B0"], borderSubtle: ["#26382C", "#D2E2D6"],
    accent: ["#8BD49C", "#1F7A3A"], secondary: ["#E6C07B", "#9A6700"],
    text: ["#DDE8DF", light.text], muted: ["#93A89A", "#61756A"],
  },
  amber: {
    bg: ["#18140E", "#FCF7EF"], panel: ["#2B2418", "#FBF1E3"], element: ["#231D14", "#F3E6D2"],
    border: ["#5A4A30", "#CDB894"], borderSubtle: ["#3A3122", "#E6D8C0"],
    accent: ["#F5B556", "#B35900"], secondary: ["#FF9B73", "#C2410C"],
    text: ["#EDE3D3", light.text], muted: ["#B0A189", "#7A6A52"],
  },
  rose: {
    bg: ["#1A1317", "#FCF3F7"], panel: ["#2A1F26", "#FAEEF3"], element: ["#241A20", "#F2E0E8"],
    border: ["#5A3C4C", "#D3AFC0"], borderSubtle: ["#3B2A33", "#EAD5DF"],
    accent: ["#F591B2", "#C2255C"], secondary: ["#C8A6F7", "#7048C8"],
    text: ["#EEDFE6", light.text], muted: ["#B3979F", "#7F6470"],
  },
  violet: {
    bg: ["#15121E", "#F6F4FD"], panel: ["#221E33", "#F0EDFB"], element: ["#1C1829", "#E4DFF6"],
    border: ["#463E66", "#B9B0D9"], borderSubtle: ["#2E2945", "#DAD4EE"],
    accent: ["#B49CFF", "#6741D9"], secondary: ["#7DD3FC", "#0369A1"],
    text: ["#E4E0F2", light.text], muted: ["#A09AB8", "#6A6485"],
  },
  mono: {
    bg: ["#0F0F0F", "#FFFFFF"], panel: ["#262626", "#EFEFEF"], element: ["#1C1C1C", "#E5E5E5"],
    border: ["#4D4D4D", "#B0B0B0"], borderSubtle: ["#333333", "#D6D6D6"],
    accent: ["#E6E6E6", "#1F2328"], secondary: ["#FFFFFF", "#000000"],
    text: ["#D9D9D9", "#1A1A1A"], muted: ["#9A9A9A", "#6B6B6B"],
  },
}

const fixed = {
  error: ["#F7768E", "#CF222E"] as Pair,
  warning: ["#E0AF68", "#9A6700"] as Pair,
  success: ["#9ECE6A", "#1A7F37"] as Pair,
  addedBg: ["#13261A", "#E6F4EA"] as Pair,
  removedBg: ["#2B1519", "#FBE9EB"] as Pair,
}

function theme(p: Palette) {
  const c = (pair: Pair) => ({ dark: pair[0], light: pair[1] })
  return {
    $schema: "https://opencode.ai/theme.json",
    theme: {
      primary: c(p.accent),
      secondary: c(p.secondary),
      accent: c(p.accent),
      error: c(fixed.error),
      warning: c(fixed.warning),
      success: c(fixed.success),
      info: c(p.accent),
      text: c(p.text),
      textMuted: c(p.muted),
      background: c(p.bg),
      backgroundPanel: c(p.panel),
      backgroundElement: c(p.element),
      border: c(p.border),
      borderActive: c(p.accent),
      borderSubtle: c(p.borderSubtle),
      diffAdded: c(fixed.success),
      diffRemoved: c(fixed.error),
      diffContext: c(p.muted),
      diffHunkHeader: c(p.muted),
      diffHighlightAdded: c(fixed.success),
      diffHighlightRemoved: c(fixed.error),
      diffAddedBg: c(fixed.addedBg),
      diffRemovedBg: c(fixed.removedBg),
      diffContextBg: c(p.bg),
      diffLineNumber: c(p.muted),
      diffAddedLineNumberBg: c(fixed.addedBg),
      diffRemovedLineNumberBg: c(fixed.removedBg),
      markdownText: c(p.text),
      markdownHeading: c(p.accent),
      markdownLink: c(p.accent),
      markdownLinkText: c(p.secondary),
      markdownCode: c(p.secondary),
      markdownBlockQuote: c(p.muted),
      markdownEmph: c(fixed.warning),
      markdownStrong: c(p.text),
      markdownHorizontalRule: c(p.border),
      markdownListItem: c(p.muted),
      markdownListEnumeration: c(p.muted),
      markdownImage: c(p.accent),
      markdownImageText: c(p.secondary),
      markdownCodeBlock: c(p.text),
      syntaxComment: c(p.muted),
      syntaxKeyword: c(p.accent),
      syntaxFunction: c(p.secondary),
      syntaxVariable: c(p.text),
      syntaxString: c(fixed.success),
      syntaxNumber: c(fixed.warning),
      syntaxType: c(p.secondary),
      syntaxOperator: c(p.muted),
      syntaxPunctuation: c(p.muted),
    },
  }
}

const out = path.resolve(import.meta.dir, "../../packages/tui/src/theme/assets")
for (const [name, p] of Object.entries(palettes)) {
  await Bun.write(path.join(out, `${name}.json`), JSON.stringify(theme(p), null, 2) + "\n")
  console.log("wrote", name)
}
