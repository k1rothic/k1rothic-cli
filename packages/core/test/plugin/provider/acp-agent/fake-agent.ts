// A stand-in ACP agent for the acp-agent tests. Conversations persist in ACP_FAKE_STATE so a new process can resume
// them, the way `agy acp` does.
import { existsSync, readFileSync, writeFileSync, appendFileSync } from "node:fs"

const state = process.env.ACP_FAKE_STATE!
const log = process.env.ACP_FAKE_LOG!
type Conversation = { turns: number; model: string }
const load = (): Record<string, Conversation> => (existsSync(state) ? JSON.parse(readFileSync(state, "utf8")) : {})
const save = (value: Record<string, Conversation>) => writeFileSync(state, JSON.stringify(value))
const MODELS = { availableModels: [{ modelId: "fast", name: "Fast" }, { modelId: "smart", name: "Smart" }] }

let next = 1000
const waiting = new Map<number, (value: any) => void>()
const send = (message: unknown) => process.stdout.write(JSON.stringify(message) + "\n")
const ask = (method: string, params: unknown) =>
  new Promise<any>((resolve) => {
    const id = next++
    waiting.set(id, resolve)
    send({ jsonrpc: "2.0", id, method, params })
  })
const update = (sessionId: string, value: unknown) =>
  send({ jsonrpc: "2.0", method: "session/update", params: { sessionId, update: value } })

const handle = async (method: string, params: any) => {
  appendFileSync(log, JSON.stringify({ method, params }) + "\n")
  const conversations = load()
  if (method === "initialize")
    return { protocolVersion: 1, agentCapabilities: { loadSession: true, sessionCapabilities: { resume: {} } } }
  if (method === "session/new") {
    const id = `acp-${Object.keys(conversations).length + 1}`
    conversations[id] = { turns: 0, model: "fast" }
    save(conversations)
    return { sessionId: id, models: { ...MODELS, currentModelId: "fast" } }
  }
  if (method === "session/resume") {
    const conversation = conversations[params.sessionId]
    if (!conversation) throw new Error("unknown session")
    conversation.model = "fast"
    save(conversations)
    return { models: { ...MODELS, currentModelId: "fast" } }
  }
  if (method === "session/set_model") {
    conversations[params.sessionId]!.model = params.modelId
    save(conversations)
    return {}
  }
  if (method === "session/prompt") {
    const conversation = conversations[params.sessionId]!
    conversation.turns++
    save(conversations)
    const text = params.prompt.map((block: any) => block.text ?? `[${block.type}]`).join("|")
    if (text.includes("use a tool")) {
      const toolCall = {
        toolCallId: "call-1",
        title: "Edit notes.md",
        kind: "edit",
        status: "pending",
        locations: [{ path: "notes.md" }],
      }
      update(params.sessionId, { sessionUpdate: "tool_call", ...toolCall })
      const answer = await ask("session/request_permission", {
        sessionId: params.sessionId,
        toolCall,
        options: [
          { optionId: "yes", name: "Allow", kind: "allow_once" },
          { optionId: "no", name: "Reject", kind: "reject_once" },
        ],
      })
      const allowed = answer.outcome.outcome === "selected" && answer.outcome.optionId === "yes"
      update(params.sessionId, {
        sessionUpdate: "tool_call_update",
        toolCallId: "call-1",
        status: allowed ? "completed" : "failed",
        content: [{ type: "content", content: { type: "text", text: allowed ? "edited" : "rejected" } }],
      })
    }
    update(params.sessionId, { sessionUpdate: "agent_thought_chunk", content: { type: "text", text: "thinking" } })
    update(params.sessionId, {
      sessionUpdate: "agent_message_chunk",
      content: { type: "text", text: `${params.sessionId} turn ${conversation.turns} on ${conversation.model}: ${text}` },
    })
    return { stopReason: "end_turn", usage: { inputTokens: 10, outputTokens: 5, cachedReadTokens: 4 } }
  }
  throw new Error(`unexpected ${method}`)
}

let buffer = ""
process.stdin.setEncoding("utf8")
process.stdin.on("data", (chunk: string) => {
  buffer += chunk
  const lines = buffer.split("\n")
  buffer = lines.pop() ?? ""
  for (const line of lines.filter((line) => line.trim())) {
    const message = JSON.parse(line)
    if (message.method === undefined) {
      waiting.get(message.id)?.(message.result)
      waiting.delete(message.id)
      continue
    }
    if (message.id === undefined) continue
    handle(message.method, message.params).then(
      (result) => send({ jsonrpc: "2.0", id: message.id, result }),
      (error: Error) => send({ jsonrpc: "2.0", id: message.id, error: { code: -32603, message: error.message } }),
    )
  }
})
