// k1rothic Mode B: launch an official vendor CLI (claude, codex, agy) in
// this terminal, unmodified, for use with the user's own subscription.
//
// Rules (do not weaken):
//  - Start the vendor CLI as a normal interactive process: no headless or
//    print flags (-p, --print, exec, --json, stream-json), no SDKs or
//    ACP/MCP bridges, no wrapping it as a backend for our agent.
//  - Never parse its output to drive our own logic (stdio is inherited).
//  - Never read, copy, store, proxy or refresh its OAuth tokens or
//    credential files, and never inject tokens via env. The vendor CLI
//    performs its own login; we only start the process.
//  - Locate binaries on PATH; if missing, show the vendor's official install
//    link. Never auto-install.
import type { CliRenderer } from "@opentui/core"
import { spawn } from "node:child_process"
import { existsSync } from "node:fs"

export type VendorID = "claude" | "codex" | "agy"

export interface Vendor {
  readonly id: VendorID
  readonly name: string
  readonly binary: string
  readonly install: string
  readonly terms: string
}

export const vendors: readonly Vendor[] = [
  {
    id: "claude",
    name: "Claude Code",
    binary: "claude",
    install: "https://code.claude.com/docs/en/setup",
    terms: "https://www.anthropic.com/legal/consumer-terms",
  },
  {
    id: "codex",
    name: "OpenAI Codex CLI",
    binary: "codex",
    install: "https://developers.openai.com/codex/cli",
    terms: "https://openai.com/policies/terms-of-use",
  },
  {
    id: "agy",
    name: "Google Antigravity",
    binary: "agy",
    install: "https://antigravity.google",
    terms: "https://policies.google.com/terms",
  },
]

/** The vendor CLI's executable on PATH (like exec.LookPath), or undefined. */
export function locate(vendor: Vendor): string | undefined {
  return Bun.which(vendor.binary) ?? undefined
}

/** The notice shown before the first launch. */
export const termsNotice =
  "Subscription plans and their terms are set by each vendor and can change at any time. " +
  "The official CLI runs unmodified and signs in by itself; k1rothic never sees its login. " +
  "Terms: " +
  vendors.map((v) => `${v.name} ${v.terms}`).join(" · ")

// Our own process variables are not passed on: the vendor CLI gets the user's
// environment as it was, and nothing is added to it.
function childEnv(): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (key.startsWith("OPENCODE_") || key.startsWith("K1ROTHIC_")) continue
    env[key] = value
  }
  return env
}

/**
 * Suspends the TUI, runs the vendor CLI with the terminal handed over
 * (inherited stdio, no arguments), and resumes the TUI when it exits.
 * Resolves with the exit code.
 */
export async function launch(input: { renderer: CliRenderer; executable: string; cwd?: string }) {
  // .cmd/.bat shims (npm installs on Windows) need cmd.exe; real executables run directly.
  const shell = process.platform === "win32" && /\.(cmd|bat)$/i.test(input.executable)
  await input.renderer.suspend()
  try {
    return await new Promise<number | null>((resolve, reject) => {
      const child = spawn(shell ? `"${input.executable}"` : input.executable, [], {
        cwd: input.cwd && existsSync(input.cwd) ? input.cwd : process.cwd(),
        stdio: "inherit",
        shell,
        env: childEnv(),
      })
      child.on("error", reject)
      child.on("exit", (code) => resolve(code))
    })
  } finally {
    await input.renderer.resume()
  }
}
