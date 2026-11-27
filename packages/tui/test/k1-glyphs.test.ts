import { expect, test } from "bun:test"

test("k1rothic safe glyphs swap font-fragile symbols for WGL4 ones", async () => {
  process.env.K1ROTHIC_GLYPHS = "safe"
  const { sym, safeGlyphs, roundedBorder } = await import(`../src/k1-glyphs.ts?safe=${Date.now()}`)
  expect(safeGlyphs).toBe(true)
  expect(sym("❯ hi ✓ ◆ Thought ⋯ ✗")).toBe("> hi √ ♦ Thought … x")
  expect(["⠋", "⠙", "⠹", "⠸"].map((f) => sym(f))).toEqual(["|", "/", "-", "\\"])
  expect(sym("plain text — ok")).toBe("plain text — ok")
  expect(roundedBorder()).toBe("single")
  // Every replacement is itself in WGL4, so applying it twice changes nothing.
  const once = sym("✱ ◈ ⚙ ↳ ⇆ △ ▶ ▸ ▾ ⚠ ◉ ↵ ┃ ╭╮╰╯")
  expect(sym(once)).toBe(once)
})

test("unicode mode leaves symbols alone", async () => {
  process.env.K1ROTHIC_GLYPHS = "unicode"
  const { sym, roundedBorder } = await import(`../src/k1-glyphs.ts?unicode=${Date.now()}`)
  expect(sym("❯ ✓ ⠋")).toBe("❯ ✓ ⠋")
  expect(roundedBorder()).toBe("rounded")
  delete process.env.K1ROTHIC_GLYPHS
})

test("the braille thinking spinner stays braille unless safe is chosen explicitly", async () => {
  delete process.env.K1ROTHIC_GLYPHS
  const { sym } = await import(`../src/k1-glyphs.ts?auto=${Date.now()}`)
  expect(sym("⠋⠙⠹")).toBe("⠋⠙⠹")
})
