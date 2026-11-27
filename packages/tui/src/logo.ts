// k1rothic wordmark in the TUI's 4-row block font. Row 0 holds ascenders;
// "_" draws a shaded cell, "^" a shaded upper half (see component/logo.tsx).
const glyphs: Record<string, string[]> = {
  k: ["█   ", "█ ▄▀", "█▀▄ ", "▀  ▀"],
  "1": [" ▄ ", "▀█ ", " █ ", "▀▀▀"],
  r: ["   ", "█▀▀", "█  ", "▀  "],
  o: ["    ", "█▀▀█", "█__█", "▀▀▀▀"],
  t: [" ▄  ", "▀█▀▀", " █  ", " ▀▀▀"],
  h: ["█   ", "█▀▀▄", "█  █", "▀  ▀"],
  i: ["▄", "▄", "█", "▀"],
  c: ["    ", "█▀▀▀", "█___", "▀▀▀▀"],
}

function word(text: string): string[] {
  return [0, 1, 2, 3].map((row) =>
    Array.from(text)
      .map((ch) => glyphs[ch]![row])
      .join(" "),
  )
}

export const logo = {
  left: word("k1"),
  right: word("rothic"),
}

// Compact mark for very narrow terminals.
export const go = {
  left: word("k"),
  right: word("k1"),
}
