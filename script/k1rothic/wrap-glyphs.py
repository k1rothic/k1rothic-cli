# Wraps user-visible text that contains font-fragile symbols in sym(...) so
# k1rothic can swap them for WGL4 symbols on fonts that lack them (see
# packages/tui/src/k1-glyphs.ts). Idempotent; re-run after upstream merges:
#
#   python script/k1rothic/wrap-glyphs.py
import json, os, re, sys

root = os.path.join(os.path.dirname(__file__), "..", "..", "packages", "tui", "src")
glyph_file = open(os.path.join(root, "k1-glyphs.ts"), encoding="utf-8").read()
chars = set(re.findall(r'"(.)": "', glyph_file))
chars |= set(re.search(r'const braille = "([^"]+)"', glyph_file).group(1))
has = lambda s: any(c in chars for c in s)

skip_dirs = ("storybook", "simulation")
skip_files = ("k1-glyphs.ts", "demo.ts")

literal = re.compile(r'''"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\]|\\.)*`''')
jsx_text = re.compile(r'(?<=[>}])([^<>{}]+)(?=[<{])')

def wrap_literals(src):
    out, last = [], 0
    for m in literal.finditer(src):
        lit = m.group(0)
        if not has(lit):
            continue
        line_start = src.rfind("\n", 0, m.start()) + 1
        line_head = src[line_start:m.start()]
        if line_head.lstrip().startswith(("//", "*", "/*")) or "//" in line_head:
            continue  # inside a comment
        before = src[max(0, m.start() - 40):m.start()]
        after = src[m.end():m.end() + 3]
        if re.search(r"(sym\($|import\s|from\s*$|case\s*$|[=!]==\s*$|[=!]=\s*$|[\w$\])]\[\s*$)", before) or after.lstrip().startswith(":") and not before.rstrip().endswith(("?", "(", ",", "=")):
            continue
        if re.search(r"[\w$]$", before) and lit.startswith("`"):
            continue  # tagged template
        if re.search(r"[\w-]=$", before):
            rep = "{sym(%s)}" % lit  # JSX attribute
        else:
            rep = "sym(%s)" % lit
        out.append(src[last:m.start()])
        out.append(rep)
        last = m.end()
    out.append(src[last:])
    return "".join(out)

def wrap_jsx(src):
    def rep(m):
        text = m.group(1)
        if not has(text) or "sym(" in text:
            return m.group(0)
        core = text.strip() if "\n" in text else text
        if not core:
            return m.group(0)
        lead = text[: len(text) - len(text.lstrip())] if "\n" in text else ""
        trail = text[len(text.rstrip()):] if "\n" in text else ""
        if any(q in core for q in "\"'`;=()"):
            return m.group(0)  # code, not JSX text
        return lead + "{sym(" + json.dumps(core, ensure_ascii=False) + ")}" + trail
    return jsx_text.sub(rep, src)

changed = 0
for dirpath, _, files in os.walk(root):
    if any(s in dirpath for s in skip_dirs):
        continue
    for f in files:
        if not f.endswith((".ts", ".tsx")) or f in skip_files:
            continue
        p = os.path.join(dirpath, f)
        src = open(p, encoding="utf-8").read()
        if not has(src):
            continue
        new = wrap_literals(src)
        if f.endswith(".tsx"):
            new = wrap_jsx(new)
        if new == src:
            continue
        if not re.search(r'import \{[^}]*\bsym\b[^}]*\} from "[./]*k1-glyphs"', new):
            rel = os.path.relpath(os.path.join(root, "k1-glyphs"), dirpath).replace(os.sep, "/")
            if not rel.startswith("."):
                rel = "./" + rel
            # Insert after the last import statement.
            imports = list(re.finditer(r'^import[^;]*?from\s+"[^"]+"\s*\n', new, re.M | re.S))
            pos = imports[-1].end() if imports else 0
            new = new[:pos] + 'import { sym } from "%s"\n' % rel + new[pos:]
        open(p, "w", encoding="utf-8", newline="\n").write(new)
        changed += 1
        print("wrapped", os.path.relpath(p, root))
print(changed, "file(s)")
