import { describe, expect, test } from "bun:test"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import os from "node:os"
import path from "node:path"
import { launch, locate, termsNotice, vendors } from "../src/official"

describe("k1rothic /official launcher", () => {
  test("knows the three official CLIs with install and terms links", () => {
    expect(vendors.map((v) => v.binary)).toEqual(["claude", "codex", "agy"])
    for (const v of vendors) {
      expect(v.install).toStartWith("https://")
      expect(v.terms).toStartWith("https://")
      expect(termsNotice).toContain(v.terms)
    }
  })

  test("locates binaries on PATH and reports missing ones", () => {
    expect(locate({ ...vendors[0]!, binary: "node" })).toBeTruthy()
    expect(locate({ ...vendors[0]!, binary: "k1rothic-definitely-not-installed" })).toBeUndefined()
  })

  test("suspends, runs the CLI with no arguments and a clean env, then resumes", async () => {
    const dir = mkdtempSync(path.join(os.tmpdir(), "k1-official-"))
    const out = path.join(dir, "out.json")
    const script = path.join(dir, "probe.js")
    writeFileSync(
      script,
      `require("fs").writeFileSync(${JSON.stringify(out)}, JSON.stringify({ argv: process.argv.slice(2), env: Object.keys(process.env).filter(k => k.startsWith("OPENCODE_") || k.startsWith("K1ROTHIC_")) })); process.exit(3)`,
    )
    // A wrapper so the "vendor binary" takes no arguments, like the real launch.
    const wrapper = path.join(dir, process.platform === "win32" ? "probe.cmd" : "probe.sh")
    writeFileSync(
      wrapper,
      process.platform === "win32"
        ? `@echo off\r\nnode "${script}" %*\r\n`
        : `#!/bin/sh\nexec node "${script}" "$@"\n`,
      { mode: 0o755 },
    )
    process.env.OPENCODE_SERVER_PASSWORD = "secret"
    process.env.K1ROTHIC_TEST = "x"
    const calls: string[] = []
    const renderer = {
      suspend: async () => void calls.push("suspend"),
      resume: async () => void calls.push("resume"),
    } as never
    const code = await launch({ renderer, executable: wrapper, cwd: dir })
    delete process.env.OPENCODE_SERVER_PASSWORD
    delete process.env.K1ROTHIC_TEST
    expect(code).toBe(3)
    expect(calls).toEqual(["suspend", "resume"])
    const seen = JSON.parse(readFileSync(out, "utf8"))
    expect(seen.argv).toEqual([])
    expect(seen.env).toEqual([])
  })
})
