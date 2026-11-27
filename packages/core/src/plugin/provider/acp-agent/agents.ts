export * as AcpAgents from "./agents.js"

import os from "node:os"
import path from "node:path"
import { existsSync } from "node:fs"
import { which } from "../../../util/which.js"
import type { AcpRpc } from "./rpc.js"

// k1rothic: subscription CLIs that speak the Agent Client Protocol. Each one becomes a provider whose chats run in a
// background process of the user's own signed-in CLI, so the vendor keeps one conversation (and its prompt cache) per
// k1rothic session.
export interface Definition {
  readonly providerID: string
  readonly name: string
  readonly integrationLabel: string
  readonly installURL: string
  readonly signIn: string
  // Undefined when the CLI is not installed
  readonly command: () => AcpRpc.Command | undefined
  // Shown before the CLI has reported its own list
  readonly models: ReadonlyArray<{ readonly id: string; readonly name: string }>
}

const home = os.homedir()

function find(name: string, fallbacks: readonly string[]) {
  const found = which(name)
  if (found) return found
  return fallbacks.find((file) => existsSync(file))
}

const exe = (file: string) => (process.platform === "win32" ? `${file}.exe` : file)
const localAppData = () => process.env.LOCALAPPDATA ?? path.join(home, "AppData", "Local")

export const antigravity: Definition = {
  providerID: "antigravity-agent",
  name: "Antigravity (subscription)",
  integrationLabel: "Antigravity login (sign in inside agy)",
  installURL: "https://antigravity.google/docs/cli",
  signIn: "Sign in to Antigravity first: run `agy` in a terminal (or /official → Antigravity → Sign in), then try again.",
  command: () => {
    const agy = find("agy", [path.join(localAppData(), "agy", "bin", exe("agy")), path.join(home, ".local", "bin", exe("agy"))])
    return agy ? { command: agy, args: ["acp"] } : undefined
  },
  models: [
    { id: "default", name: "Default (agy setting)" },
    { id: "gemini-4-hydrogen-high", name: "Gemini 4 Hydrogen (High)" },
    { id: "gemini-4-hydrogen-medium", name: "Gemini 4 Hydrogen (Medium)" },
    { id: "gemini-3.8-flash-high", name: "Gemini 3.8 Flash (High)" },
    { id: "gemini-3.8-flash-low", name: "Gemini 3.8 Flash (Low)" },
  ],
}

export const codex: Definition = {
  providerID: "codex-agent",
  name: "Codex (ChatGPT subscription)",
  integrationLabel: "ChatGPT login (codex login)",
  installURL: "https://developers.openai.com/codex/cli",
  signIn: "Sign in to Codex first: run `codex login` in a terminal (or /official → Codex → Sign in), then try again.",
  command: () => {
    const codexBin = find("codex", [
      path.join(localAppData(), "Programs", "OpenAI", "Codex", "bin", exe("codex")),
      path.join(home, ".local", "bin", exe("codex")),
    ])
    if (!codexBin) return undefined
    const env = { CODEX_PATH: codexBin }
    // The ACP adapter for Codex is a separate program; npx fetches it on first use when it isn't installed.
    const adapter = which("codex-acp")
    if (adapter) return { command: adapter, args: [], env }
    const npx = which("npx")
    if (!npx) return undefined
    return { command: npx, args: ["-y", "@agentclientprotocol/codex-acp"], env }
  },
  models: [{ id: "default", name: "Default (codex setting)" }],
}

export const all = [antigravity, codex] as const
