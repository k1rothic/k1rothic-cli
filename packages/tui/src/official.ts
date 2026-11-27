// k1rothic Mode B: launch an official vendor CLI (claude, codex, agy) in
// this terminal, unmodified, for use with the user's own subscription.
//
// Rules (do not weaken):
//  - Start the vendor CLI as a normal interactive process: no headless or
//    print flags (-p, --print, exec, --json, stream-json), no SDKs or
//    ACP/MCP bridges, no wrapping it as a backend for our agent. The only
//    arguments ever passed are the vendor's own interactive flags for the
//    model, continuing the last conversation, and its sign-in subcommand.
//  - Never parse its output to drive our own logic (stdio is inherited).
//  - Never read, copy, store, proxy or refresh its OAuth tokens or
//    credential files, and never inject tokens via env. The vendor CLI
//    performs its own login; we only start the process.
//  - Locate binaries on PATH (or the official installer's default folder).
//    Installing only happens when the user picks "Install" and confirms; it
//    runs the vendor's official installer visibly in the terminal.
import type { CliRenderer } from "@opentui/core"
import { spawn } from "node:child_process"
import { existsSync } from "node:fs"
import os from "node:os"
import path from "node:path"

export type VendorID = "claude" | "codex" | "agy"

export interface VendorModel {
  readonly id: string
  readonly title: string
  readonly description?: string
}

export interface Vendor {
  readonly id: VendorID
  readonly name: string
  readonly binary: string
  readonly plan: string
  readonly install: string
  readonly terms: string
  /** The vendor's official one-line installer, run only after the user confirms. */
  readonly installer: { readonly win32: string; readonly posix: string }
  /** Where the official installer puts the binary, for when PATH is not refreshed yet. */
  readonly installDirs: (home: string, env: NodeJS.ProcessEnv) => string[]
  /** Arguments for the vendor's own interactive sign-in. Empty: the CLI asks on first start. */
  readonly signIn: readonly string[]
  /** Arguments that reopen the most recent conversation interactively. */
  readonly resume: readonly string[]
  readonly models: readonly VendorModel[]
}

export const vendors: readonly Vendor[] = [
  {
    id: "claude",
    name: "Claude Code",
    binary: "claude",
    plan: "Claude Pro / Max",
    install: "https://code.claude.com/docs/en/setup",
    terms: "https://www.anthropic.com/legal/consumer-terms",
    installer: {
      win32: "irm https://claude.ai/install.ps1 | iex",
      posix: "curl -fsSL https://claude.ai/install.sh | bash",
    },
    installDirs: (home) => [path.join(home, ".local", "bin")],
    signIn: [],
    resume: ["--continue"],
    models: [
      { id: "opus", title: "Opus", description: "latest Opus · complex work" },
      { id: "sonnet", title: "Sonnet", description: "latest Sonnet · everyday coding" },
      { id: "haiku", title: "Haiku", description: "latest Haiku · fast and light" },
      { id: "fable", title: "Fable", description: "hardest, longest-running tasks" },
      { id: "opusplan", title: "Opus plan → Sonnet", description: "Opus in plan mode, Sonnet to execute" },
    ],
  },
  {
    id: "codex",
    name: "OpenAI Codex CLI",
    binary: "codex",
    plan: "ChatGPT Plus / Pro",
    install: "https://developers.openai.com/codex/cli",
    terms: "https://openai.com/policies/terms-of-use",
    installer: {
      win32: "irm https://chatgpt.com/codex/install.ps1 | iex",
      posix: "curl -fsSL https://chatgpt.com/codex/install.sh | sh",
    },
    installDirs: (home, env) => [
      path.join(env.LOCALAPPDATA ?? path.join(home, "AppData", "Local"), "Programs", "OpenAI", "Codex", "bin"),
      path.join(home, ".local", "bin"),
    ],
    signIn: ["login"],
    resume: ["resume", "--last"],
    models: [
      { id: "gpt-6.2-astra", title: "GPT-6.2 Astra", description: "most capable" },
      { id: "gpt-6.2-sol", title: "GPT-6.2 Sol", description: "everyday coding" },
      { id: "gpt-6.1-sol", title: "GPT-6.1 Sol", description: "complex work, lower cost" },
      { id: "gpt-6.1-terra", title: "GPT-6.1 Terra", description: "fast and efficient" },
      { id: "gpt-6-luna", title: "GPT-6 Luna", description: "focused, high-volume tasks" },
    ],
  },
  {
    id: "agy",
    name: "Google Antigravity",
    binary: "agy",
    plan: "Google AI Pro / Ultra",
    install: "https://antigravity.google/docs/getting-started/",
    terms: "https://policies.google.com/terms",
    installer: {
      win32: "irm https://antigravity.google/cli/install.ps1 | iex",
      posix: "curl -fsSL https://antigravity.google/cli/install.sh | bash",
    },
    installDirs: (home, env) => [
      path.join(env.LOCALAPPDATA ?? path.join(home, "AppData", "Local"), "agy", "bin"),
      path.join(home, ".local", "bin"),
    ],
    signIn: [],
    resume: ["--continue"],
    models: [
      { id: "gemini-4-neon-high", title: "Gemini 4 Neon (High)" },
      { id: "gemini-4-carbon-high", title: "Gemini 4 Carbon (High)" },
      { id: "gemini-4-argon-high", title: "Gemini 4 Argon (High)" },
      { id: "gemini-4-hydrogen-high", title: "Gemini 4 Hydrogen (High)" },
      { id: "gemini-3.8-flash-high", title: "Gemini 3.8 Flash (High)" },
    ],
  },
]

/** The vendor CLI's executable on PATH or in its installer's default folder, or undefined. */
export function locate(vendor: Vendor) {
  const found = Bun.which(vendor.binary)
  if (found) return found
  const names = process.platform === "win32" ? [`${vendor.binary}.exe`, `${vendor.binary}.cmd`] : [vendor.binary]
  return vendor
    .installDirs(os.homedir(), process.env)
    .flatMap((dir) => names.map((name) => path.join(dir, name)))
    .find((file) => existsSync(file))
}

/** The notice shown before the first launch. */
export const termsNotice =
  "Subscription plans and their terms are set by each vendor and can change at any time. " +
  "The official CLI runs unmodified and signs in by itself; k1rothic never sees its login. " +
  "Terms: " +
  vendors.map((v) => `${v.name} ${v.terms}`).join(" · ")

/** Shown when the model changes inside an ongoing conversation. */
export const cacheNotice =
  "Prompt caches belong to one model. Continuing a conversation on a different model drops the cache: " +
  "the next message re-sends the whole history uncached, so it is slower and uses more of your plan limits. " +
  "The conversation itself is kept."

// Model ids and labels such as "opus", "gpt-6.1-sol" or "Gemini 3.7 Flash (Low)". No leading dash (it
// would read as a flag) and no quotes or shell metacharacters (Windows .cmd shims run through cmd.exe).
const MODEL_ID = /^[A-Za-z0-9][A-Za-z0-9._:/@[\] ()-]*$/

export function validModel(model: string) {
  return model.length <= 120 && MODEL_ID.test(model)
}

/** Interactive launch arguments: the model flag and, optionally, the vendor's "continue last conversation". */
export function launchArgs(vendor: Vendor, input: { model?: string; resume?: boolean }) {
  const model = input.model?.trim()
  if (model && !validModel(model)) throw new Error(`Invalid model name: ${model}`)
  return [...(model ? ["--model", model] : []), ...(input.resume ? vendor.resume : [])]
}

// Our own process variables are not passed on: the vendor CLI gets the user's
// environment as it was, and nothing is added to it.
function childEnv() {
  return Object.fromEntries(
    Object.entries(process.env).filter(([key]) => !key.startsWith("OPENCODE_") && !key.startsWith("K1ROTHIC_")),
  )
}

/**
 * Suspends the TUI, runs the vendor CLI with the terminal handed over
 * (inherited stdio), and resumes the TUI when it exits. Resolves with the exit code.
 */
export function launch(input: { renderer: CliRenderer; executable: string; args?: readonly string[]; cwd?: string }) {
  // .cmd/.bat shims (npm installs on Windows) need cmd.exe; real executables run directly.
  const shell = process.platform === "win32" && /\.(cmd|bat)$/i.test(input.executable)
  const args = input.args ?? []
  return run(input.renderer, () =>
    spawn(shell ? `"${input.executable}"` : input.executable, shell ? args.map((arg) => `"${arg}"`) : args, {
      cwd: input.cwd && existsSync(input.cwd) ? input.cwd : process.cwd(),
      stdio: "inherit",
      shell,
      env: childEnv(),
    }),
  )
}

/** Runs the vendor's official installer in the handed-over terminal and waits for Enter before resuming. */
export function install(input: { renderer: CliRenderer; vendor: Vendor }) {
  const pause = "Installer finished. Press Enter to return to k1rothic"
  return run(input.renderer, () =>
    process.platform === "win32"
      ? spawn(
          "powershell.exe",
          [
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-Command",
            `${input.vendor.installer.win32}; Read-Host '${pause}'`,
          ],
          { stdio: "inherit", env: childEnv() },
        )
      : spawn("sh", ["-c", `${input.vendor.installer.posix}; printf '\\n${pause} '; read _`], {
          stdio: "inherit",
          env: childEnv(),
        }),
  )
}

async function run(renderer: CliRenderer, start: () => ReturnType<typeof spawn>) {
  await renderer.suspend()
  try {
    return await new Promise<number | null>((resolve, reject) => {
      const child = start()
      child.on("error", reject)
      child.on("exit", (code) => resolve(code))
    })
  } finally {
    await renderer.resume()
  }
}
