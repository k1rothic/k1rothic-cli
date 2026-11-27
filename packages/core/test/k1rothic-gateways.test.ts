import { describe, expect, test } from "bun:test"
import fs from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { configFile, gatewayID, listGateways, removeGateway, saveGateway } from "../src/k1rothic-gateways"
import { allowedByDefault, DEFAULT_PROVIDERS } from "../src/k1rothic-providers"

describe("k1rothic gateways", () => {
  test("ids avoid built-in providers and each other", () => {
    expect(gatewayID("My LiteLLM")).toBe("my-litellm")
    expect(gatewayID("OpenAI")).toBe("openai-custom")
    expect(gatewayID("Work", new Set(["work"]))).toBe("work-2")
    expect(gatewayID("!!!")).toBe("gateway")
  })

  test("saves, lists and removes gateways while keeping existing config and comments", async () => {
    const dir = await fs.mkdtemp(path.join(os.tmpdir(), "k1-gw-"))
    await fs.writeFile(path.join(dir, "k1rothic.jsonc"), '{\n  // my settings\n  "shell": "pwsh"\n}\n')
    expect(await configFile(dir)).toBe(path.join(dir, "k1rothic.jsonc"))

    await saveGateway(dir, {
      id: "deepseek-gw",
      name: "DeepSeek GW",
      api: "anthropic-messages",
      baseURL: "https://api.deepseek.com/anthropic/",
      models: ["deepseek-chat", "deepseek-reasoner"],
    })
    await saveGateway(dir, {
      id: "local",
      name: "Local",
      api: "chat-completions",
      baseURL: "http://localhost:11434/v1",
      models: ["llama3.3"],
    })
    const text = await fs.readFile(path.join(dir, "k1rothic.jsonc"), "utf8")
    expect(text).toContain("// my settings")
    expect(text).toContain('"shell": "pwsh"')
    expect(text).toContain("@opencode/ai/providers/anthropic-compatible")
    expect(text).not.toMatch(/apiKey|"key"/)

    const list = await listGateways(dir)
    expect(list).toEqual([
      {
        id: "deepseek-gw",
        name: "DeepSeek GW",
        api: "anthropic-messages",
        baseURL: "https://api.deepseek.com/anthropic",
        models: ["deepseek-chat", "deepseek-reasoner"],
      },
      { id: "local", name: "Local", api: "chat-completions", baseURL: "http://localhost:11434/v1", models: ["llama3.3"] },
    ])

    await removeGateway(dir, "deepseek-gw")
    expect((await listGateways(dir)).map((g) => g.id)).toEqual(["local"])
  })

  test("default allowlist keeps the curated providers and the user's own", () => {
    for (const id of ["anthropic", "openai", "openrouter", "google", "ollama"]) expect(DEFAULT_PROVIDERS.has(id)).toBe(true)
    expect(allowedByDefault("togetherai", new Set())).toBe(false)
    expect(allowedByDefault("my-gw", new Set(["my-gw"]))).toBe(true)
  })
})
