import { describe, expect, test } from "bun:test"
import path from "node:path"
import type { LanguageModelV3Prompt, LanguageModelV3StreamPart } from "@ai-sdk/provider"
import type { AcpAgents } from "../../../../src/plugin/provider/acp-agent/agents"
import { AcpLanguageModel } from "../../../../src/plugin/provider/acp-agent/language-model"
import { AcpPermission } from "../../../../src/plugin/provider/acp-agent/permission"
import { AcpPrompt } from "../../../../src/plugin/provider/acp-agent/prompt"
import { AcpSessions } from "../../../../src/plugin/provider/acp-agent/sessions"
import { tmpdir } from "../../../fixture/tmpdir"

const user = (text: string) => ({ role: "user" as const, content: [{ type: "text" as const, text }] })
const assistant = (text: string) => ({ role: "assistant" as const, content: [{ type: "text" as const, text }] })

async function setup(dir: string, answer: (toolCall: Record<string, unknown>) => Promise<boolean> = async () => true) {
  const definition: AcpAgents.Definition = {
    providerID: "fake-agent",
    name: "Fake",
    integrationLabel: "Fake login",
    installURL: "https://example.com",
    signIn: "Sign in to Fake",
    command: () => ({
      command: process.execPath,
      args: [path.join(import.meta.dir, "fake-agent.ts")],
      env: { ACP_FAKE_STATE: path.join(dir, "state.json"), ACP_FAKE_LOG: path.join(dir, "log.jsonl") },
    }),
    models: [],
  }
  const saved = new Map<string, string>()
  const listed: string[][] = []
  const sessions = AcpSessions.make({
    definition,
    directory: dir,
    load: async (id) => saved.get(id),
    save: async (id, acp) => void saved.set(id, acp),
    permission: (input) => answer(input.toolCall),
    models: (models) => listed.push(models.map((model) => model.id)),
  })
  const turn = async (model: string, prompt: LanguageModelV3Prompt, kind = "primary") => {
    const language = new AcpLanguageModel.Model("fake-agent", model, "fake", sessions)
    const result = await language.doStream({
      prompt,
      headers: { [AcpLanguageModel.KIND_HEADER]: kind, [AcpLanguageModel.SESSION_HEADER]: "ses_1" },
    })
    const parts: LanguageModelV3StreamPart[] = []
    for await (const part of result.stream) parts.push(part)
    return {
      parts,
      text: parts.flatMap((part) => (part.type === "text-delta" ? [part.delta] : [])).join(""),
    }
  }
  const methods = async () =>
    (await Bun.file(path.join(dir, "log.jsonl")).text())
      .trim()
      .split("\n")
      .map((line) => JSON.parse(line).method as string)
  return { sessions, turn, saved, listed, methods }
}

describe("AcpSessions", () => {
  test("keeps one agent conversation per chat and sends only the new input", async () => {
    await using dir = await tmpdir()
    const fake = await setup(dir.path)
    const first = await fake.turn("default", [user("hello")])
    expect(first.text).toBe("acp-1 turn 1 on fast: hello")
    const second = await fake.turn("default", [user("hello"), assistant(first.text), user("again")])
    expect(second.text).toBe("acp-1 turn 2 on fast: again")
    expect(fake.saved.get("ses_1")).toBe("acp-1")
    expect(fake.listed[0]).toEqual(["fast", "smart"])
    const finish = second.parts.find((part) => part.type === "finish")
    expect(finish?.type === "finish" && finish.usage.inputTokens).toEqual({
      total: 10,
      noCache: 6,
      cacheRead: 4,
      cacheWrite: undefined,
    })
    fake.sessions.close()
  })

  test("resumes the saved conversation in a new process and applies the model again", async () => {
    await using dir = await tmpdir()
    const fake = await setup(dir.path)
    await fake.turn("smart", [user("one")])
    fake.sessions.close()
    const resumed = await fake.turn("smart", [user("one"), assistant("ok"), user("two")])
    expect(resumed.text).toBe("acp-1 turn 2 on smart: two")
    expect(await fake.methods()).toEqual([
      "initialize",
      "session/new",
      "session/set_model",
      "session/prompt",
      "initialize",
      "session/resume",
      "session/set_model",
      "session/prompt",
    ])
    fake.sessions.close()
  })

  test("asks k1rothic before the agent's tools run and shows them as agent-run calls", async () => {
    await using dir = await tmpdir()
    const asked: Record<string, unknown>[] = []
    const fake = await setup(dir.path, async (toolCall) => {
      asked.push(toolCall)
      return false
    })
    const result = await fake.turn("default", [user("please use a tool")])
    expect(asked.map((call) => call.title)).toEqual(["Edit notes.md"])
    const call = result.parts.find((part) => part.type === "tool-call")
    expect(call).toMatchObject({ toolName: "fake_edit", providerExecuted: true })
    const output = result.parts.find((part) => part.type === "tool-result")
    expect(output).toMatchObject({ toolCallId: "call-1", isError: true, result: { type: "error", value: "rejected" } })
    expect(result.parts.some((part) => part.type === "reasoning-delta")).toBe(true)
    fake.sessions.close()
  })

  test("runs side calls in a throwaway conversation", async () => {
    await using dir = await tmpdir()
    const fake = await setup(dir.path)
    const result = await fake.turn("default", [{ role: "system", content: "Summarize" }, user("hi")], "generate")
    expect(result.text).toContain("Instructions:\nSummarize")
    expect(fake.saved.size).toBe(0)
  })
})

describe("AcpPrompt", () => {
  test("a new agent conversation gets the earlier messages as a transcript", () => {
    const blocks = AcpPrompt.latest([user("first"), assistant("reply"), user("next")], true)
    expect(blocks).toHaveLength(2)
    expect(blocks[0]).toMatchObject({ type: "text" })
    expect(blocks[0]?.type === "text" && blocks[0].text).toContain("User: first\n\nAssistant: reply")
    expect(blocks[1]).toEqual({ type: "text", text: "next" })
    expect(AcpPrompt.latest([user("first"), assistant("reply"), user("next")], false)).toEqual([
      { type: "text", text: "next" },
    ])
  })
})

describe("AcpPermission", () => {
  test("maps agent tools onto k1rothic's permission actions", () => {
    const dir = path.resolve("project")
    expect(
      AcpPermission.describe({ kind: "edit", title: "Edit", locations: [{ path: path.join(dir, "src", "a.ts") }] }, dir),
    ).toMatchObject({ action: "edit", resources: ["src/a.ts"] })
    expect(AcpPermission.describe({ kind: "execute", title: "ls", rawInput: { command: "ls -la" } }, dir)).toMatchObject(
      { action: "shell", resources: ["ls -la"] },
    )
    expect(AcpPermission.describe({ kind: "think", title: "Plan" }, dir)).toMatchObject({
      action: "agent_tool",
      resources: ["Plan"],
    })
  })
})
