// k1rothic: custom API gateways, stored as custom providers under
// `providers` in the user's global config file (k1rothic.json(c), or an
// existing opencode.json(c)). API keys are NOT stored here: they go through
// the credential store, which keeps them in the OS keychain.
import fs from "node:fs/promises"
import path from "node:path"
import { applyEdits, modify, parse } from "jsonc-parser"
import { names } from "./config/discovery.js"
import { DEFAULT_PROVIDERS } from "./k1rothic-providers.js"

export type GatewayAPI = "chat-completions" | "responses" | "anthropic-messages"

export const gatewayAPIs: readonly { id: GatewayAPI; label: string; hint: string; example: string }[] = [
  {
    id: "chat-completions",
    label: "OpenAI-compatible (Chat Completions)",
    hint: "Ollama, vLLM, LM Studio, LiteLLM, OpenRouter, Groq, Together, DeepSeek…",
    example: "http://localhost:11434/v1",
  },
  {
    id: "responses",
    label: "OpenAI Responses",
    hint: "OpenAI's newer API; also vLLM, OpenRouter, Azure",
    example: "https://api.example.com/v1",
  },
  {
    id: "anthropic-messages",
    label: "Anthropic-compatible (Messages)",
    hint: "Claude-compatible gateways: DeepSeek, Kimi, MiniMax, GLM, LiteLLM…",
    example: "https://api.deepseek.com/anthropic",
  },
]

const packages: Record<GatewayAPI, string> = {
  "chat-completions": "@opencode/ai/providers/openai-compatible",
  responses: "@opencode/ai/providers/openai-compatible-responses",
  "anthropic-messages": "@opencode/ai/providers/anthropic-compatible",
}

export interface Gateway {
  id: string
  name: string
  api: GatewayAPI
  baseURL: string
  models: string[]
}

const format = { formattingOptions: { tabSize: 2, insertSpaces: true } }

/** The config file gateways are written to: the last existing candidate, else k1rothic.jsonc. */
export async function configFile(directory: string) {
  let found: string | undefined
  for (const name of names) {
    const file = path.join(directory, name)
    if (await fs.stat(file).then((s) => s.isFile()).catch(() => false)) found = file
  }
  return found ?? path.join(directory, "k1rothic.jsonc")
}

/** A provider id for a gateway name that does not collide with built-in providers. */
export function gatewayID(name: string, taken: ReadonlySet<string> = new Set()) {
  let id = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
  if (!id) id = "gateway"
  if (DEFAULT_PROVIDERS.has(id)) id = `${id}-custom`
  let candidate = id
  for (let i = 2; taken.has(candidate); i++) candidate = `${id}-${i}`
  return candidate
}

function apiOf(pkg: unknown): GatewayAPI | undefined {
  for (const [api, name] of Object.entries(packages)) if (name === pkg) return api as GatewayAPI
  return undefined
}

/** Gateways (custom providers with a k1rothic API package) in the global config. */
export async function listGateways(directory: string): Promise<Gateway[]> {
  const file = await configFile(directory)
  const text = await fs.readFile(file, "utf8").catch(() => "")
  const config = (parse(text) ?? {}) as { providers?: Record<string, Record<string, unknown>> }
  return Object.entries(config.providers ?? {}).flatMap(([id, info]) => {
    const api = apiOf(info.package)
    if (!api) return []
    const settings = (info.settings ?? {}) as { baseURL?: string }
    return [
      {
        id,
        name: typeof info.name === "string" ? info.name : id,
        api,
        baseURL: settings.baseURL ?? "",
        models: Object.keys((info.models ?? {}) as Record<string, unknown>),
      },
    ]
  })
}

async function edit(directory: string, pathParts: (string | number)[], value: unknown) {
  const file = await configFile(directory)
  const text = (await fs.readFile(file, "utf8").catch(() => "")) || "{}\n"
  const updated = applyEdits(text, modify(text, pathParts, value, format))
  await fs.mkdir(path.dirname(file), { recursive: true })
  await fs.writeFile(file, updated.endsWith("\n") ? updated : `${updated}\n`)
  return file
}

/** Adds or replaces a gateway. Returns the config file written. */
export function saveGateway(directory: string, gateway: Gateway) {
  return edit(directory, ["providers", gateway.id], {
    name: gateway.name,
    package: packages[gateway.api],
    settings: { baseURL: gateway.baseURL.replace(/\/+$/, "") },
    models: Object.fromEntries(gateway.models.map((model) => [model, { name: model }])),
  })
}

/** Removes a gateway from the config (its key is removed separately). */
export function removeGateway(directory: string, id: string) {
  return edit(directory, ["providers", id], undefined)
}

/** Asks the gateway for its models (GET /models, or /v1/models for Anthropic-compatible). */
export async function fetchGatewayModels(api: GatewayAPI, baseURL: string, key?: string): Promise<string[]> {
  const base = baseURL.replace(/\/+$/, "")
  const url = api === "anthropic-messages" ? `${base}/v1/models` : `${base}/models`
  const headers: Record<string, string> = {}
  if (key) {
    headers.Authorization = `Bearer ${key}`
    if (api === "anthropic-messages") {
      headers["x-api-key"] = key
      headers["anthropic-version"] = "2023-06-01"
    }
  }
  const response = await fetch(url, { headers, signal: AbortSignal.timeout(15_000) })
  if (!response.ok) throw new Error(`${url} answered HTTP ${response.status}`)
  const body = (await response.json()) as { data?: { id?: unknown }[]; models?: { id?: unknown; name?: unknown }[] }
  const list = body.data ?? body.models ?? []
  return list
    .map((item) => (typeof item.id === "string" ? item.id : typeof (item as { name?: unknown }).name === "string" ? (item as { name: string }).name : ""))
    .filter((id) => id !== "")
    .slice(0, 300)
}
