// Rebrands user-visible "opencode"/"OpenCode" text inside string literals of
// the TUI and CLI. Code identifiers (OpenCode.make, @opencode/* imports),
// internal ids ("opencode.sidebar.*"), env vars (OPENCODE_*) and URLs are
// left alone so upstream merges stay manageable.
//
//   bun script/k1rothic/rebrand-strings.ts [--check]
import { Glob } from "bun"
import path from "path"

const root = path.resolve(import.meta.dir, "../..")
const dirs = ["packages/tui/src", "packages/cli/src"]
const check = process.argv.includes("--check")

const literal = /"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\]|\\.)*`/g

function rebrand(text: string): string {
  return text
    .replace(/\bOpenCode\b(?! (?:Go|Console|Zen)\b)(?![.\w-])/g, "k1rothic")
    .replace(/\bopencode(?= (?:run|mini|-s|--session|upgrade|models|pair|serve|auth|↵|ses_))/g, "k1rothic")
    .replace(/\}opencode\$\{/g, "}k1rothic${")
    .replace(/\(opencode\.json\)/g, "(k1rothic.json)")
}

let changed = 0
for (const dir of dirs) {
  for await (const rel of new Glob("**/*.{ts,tsx}").scan(path.join(root, dir))) {
    if (rel.includes(".test.") || rel.includes("storybook")) continue
    const file = path.join(root, dir, rel)
    const src = await Bun.file(file).text()
    const out = src.replace(literal, (lit) => rebrand(lit))
    if (out !== src) {
      changed++
      console.log((check ? "would change " : "changed ") + path.join(dir, rel))
      if (!check) await Bun.write(file, out)
    }
  }
}
console.log(`${changed} file(s)`)
