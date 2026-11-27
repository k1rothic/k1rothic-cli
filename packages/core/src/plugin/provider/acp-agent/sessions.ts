export * as AcpSessions from "./sessions.js"

import type { LanguageModelV3Prompt } from "@ai-sdk/provider"
import type { AcpAgents } from "./agents.js"
import { AcpPrompt } from "./prompt.js"
import { AcpRpc } from "./rpc.js"

// A chat's agent process stays up between turns so the vendor keeps its conversation and prompt cache warm. After this
// long without a turn it stops; the next turn starts a new process and resumes the same conversation.
export const IDLE_MS = 10 * 60_000

// JSON-RPC code ACP agents use when the user has to sign in first.
const AUTH_REQUIRED = -32000

export interface Request {
  // Only the chat's own requests have one. Side calls (titles, summaries) run in a throwaway conversation.
  readonly sessionID?: string
  readonly model: string
  readonly prompt: LanguageModelV3Prompt
  readonly emit: (update: Record<string, unknown>) => void
}

export interface Result {
  readonly stopReason: string
  readonly usage?: Record<string, unknown>
}

export interface PermissionRequest {
  readonly sessionID: string
  readonly toolCall: Record<string, unknown>
  readonly signal: AbortSignal
}

export interface Dependencies {
  readonly definition: AcpAgents.Definition
  readonly directory: string
  readonly load: (sessionID: string) => Promise<string | undefined>
  readonly save: (sessionID: string, acpSessionID: string) => Promise<void>
  // Resolves to whether the user allowed the call. Rejects when the user declined without feedback and the turn should stop.
  readonly permission: (input: PermissionRequest) => Promise<boolean>
  readonly models: (models: ReadonlyArray<{ readonly id: string; readonly name: string }>) => void
}

type ModelControl =
  | { readonly type: "models" }
  | { readonly type: "config"; readonly configID: string }
  | { readonly type: "none" }

interface Live {
  readonly connection: AcpRpc.Connection
  readonly capabilities: Record<string, unknown>
}

interface Conversation {
  readonly id: string
  readonly control: ModelControl
  readonly initialModel?: string
  model?: string
}

interface Entry {
  queue: Promise<unknown>
  live?: Live
  conversation?: Conversation
  idle?: ReturnType<typeof setTimeout>
  active?: { readonly sessionID: string; readonly signal: AbortSignal }
}

export interface Interface {
  readonly run: (request: Request, signal: AbortSignal) => Promise<Result>
  readonly remove: (sessionID: string) => void
  readonly close: () => void
}

export function make(deps: Dependencies): Interface {
  const entries = new Map<string, Entry>()

  const stop = (entry: Entry) => {
    entry.live?.connection.close()
    entry.live = undefined
    entry.conversation = undefined
    if (entry.idle) clearTimeout(entry.idle)
    entry.idle = undefined
  }

  const spawn = async (entry: Entry): Promise<Live> => {
    if (entry.live?.connection.running()) return entry.live
    entry.conversation = undefined
    const command = deps.definition.command()
    if (!command) throw new Error(`${deps.definition.name} is not installed. See ${deps.definition.installURL}`)
    const connection = await AcpRpc.start({ ...command, cwd: deps.directory })
    connection.handle("session/request_permission", (params) => answerPermission(entry, params))
    const initialized = await Promise.race([
      connection.request("initialize", {
        protocolVersion: 1,
        clientCapabilities: { fs: { readTextFile: false, writeTextFile: false }, terminal: false },
        clientInfo: { name: "k1rothic", version: "1" },
      }),
      connection.exited.then((reason) => Promise.reject(new Error(reason))),
    ]).catch((error: unknown) => {
      connection.close()
      throw error
    })
    const info = AcpRpc.isRecord(initialized) ? initialized : {}
    const capabilities = AcpRpc.isRecord(info.agentCapabilities) ? info.agentCapabilities : {}
    entry.live = { connection, capabilities }
    return entry.live
  }

  const answerPermission = async (entry: Entry, params: Record<string, unknown>) => {
    const options = Array.isArray(params.options) ? params.options.filter(AcpRpc.isRecord) : []
    const pick = (kinds: readonly string[]) => {
      const option = options.find((item) => typeof item.kind === "string" && kinds.includes(item.kind))
      return option && typeof option.optionId === "string"
        ? { outcome: { outcome: "selected", optionId: option.optionId } }
        : { outcome: { outcome: "cancelled" } }
    }
    const active = entry.active
    // Side calls have nobody to ask, so their tools stay off.
    if (!active || active.signal.aborted) return pick(["reject_once", "reject_always"])
    const toolCall = AcpRpc.isRecord(params.toolCall) ? params.toolCall : {}
    const allowed = await deps
      .permission({ sessionID: active.sessionID, toolCall, signal: active.signal })
      .catch(() => undefined)
    if (allowed === undefined) {
      // A plain decline ends the turn, like k1rothic's own tools.
      const conversation = entry.conversation
      if (conversation) entry.live?.connection.notify("session/cancel", { sessionId: conversation.id })
      return { outcome: { outcome: "cancelled" } }
    }
    return allowed ? pick(["allow_once", "allow_always"]) : pick(["reject_once", "reject_always"])
  }

  const open = async (entry: Entry, live: Live, sessionID: string | undefined) => {
    if (entry.conversation) return { conversation: entry.conversation, fresh: false }
    const saved = sessionID ? await deps.load(sessionID) : undefined
    const resumed = saved ? await resume(live, saved).catch(() => undefined) : undefined
    if (saved && resumed) {
      entry.conversation = conversation(saved, resumed)
      return { conversation: entry.conversation, fresh: false }
    }
    const created = await live.connection
      .request("session/new", { cwd: deps.directory, mcpServers: [] })
      .catch((error: unknown) => {
        if (error instanceof AcpRpc.RpcError && error.code === AUTH_REQUIRED) throw new Error(deps.definition.signIn)
        throw error
      })
    const response = AcpRpc.isRecord(created) ? created : {}
    if (typeof response.sessionId !== "string") throw new Error(`${deps.definition.name} did not start a conversation`)
    entry.conversation = conversation(response.sessionId, response)
    if (sessionID) await deps.save(sessionID, response.sessionId)
    return { conversation: entry.conversation, fresh: true }
  }

  // Prefers resume, which reconnects without replaying the history; load replays it as updates nobody listens to.
  const resume = async (live: Live, id: string) => {
    const sessions = AcpRpc.isRecord(live.capabilities.sessionCapabilities) ? live.capabilities.sessionCapabilities : {}
    const params = { sessionId: id, cwd: deps.directory, mcpServers: [] }
    if (AcpRpc.isRecord(sessions.resume)) return record(await live.connection.request("session/resume", params))
    if (live.capabilities.loadSession === true) return record(await live.connection.request("session/load", params))
    return undefined
  }

  const conversation = (id: string, response: Record<string, unknown>): Conversation => {
    const listed = listModels(response)
    if (listed.models.length > 0) deps.models(listed.models)
    return { id, control: listed.control, initialModel: listed.current }
  }

  const applyModel = async (live: Live, conversation: Conversation, model: string) => {
    const target = model === "default" ? conversation.initialModel : model
    if (!target || conversation.control.type === "none") return
    // A resumed conversation starts on the CLI's default model again, so the model is set on every new process.
    if ((conversation.model ?? conversation.initialModel) === target) return
    if (conversation.control.type === "models")
      await live.connection.request("session/set_model", { sessionId: conversation.id, modelId: target })
    if (conversation.control.type === "config")
      await live.connection.request("session/set_config_option", {
        sessionId: conversation.id,
        configId: conversation.control.configID,
        value: target,
      })
    conversation.model = target
  }

  const turn = async (entry: Entry, request: Request, signal: AbortSignal): Promise<Result> => {
    if (entry.idle) clearTimeout(entry.idle)
    entry.idle = undefined
    const live = await spawn(entry)
    const opened = await open(entry, live, request.sessionID)
    await applyModel(live, opened.conversation, request.model)
    const prompt = request.sessionID
      ? AcpPrompt.latest(request.prompt, opened.fresh)
      : AcpPrompt.standalone(request.prompt)
    const id = opened.conversation.id
    const unsubscribe = live.connection.onNotification((method, params) => {
      if (method !== "session/update" || params.sessionId !== id || !AcpRpc.isRecord(params.update)) return
      request.emit(params.update)
    })
    const cancel = () => live.connection.notify("session/cancel", { sessionId: id })
    signal.addEventListener("abort", cancel, { once: true })
    entry.active = request.sessionID ? { sessionID: request.sessionID, signal } : undefined
    try {
      const result = record(await live.connection.request("session/prompt", { sessionId: id, prompt }))
      return {
        stopReason: typeof result.stopReason === "string" ? result.stopReason : "end_turn",
        usage: AcpRpc.isRecord(result.usage) ? result.usage : undefined,
      }
    } finally {
      entry.active = undefined
      signal.removeEventListener("abort", cancel)
      unsubscribe()
    }
  }

  const run = (request: Request, signal: AbortSignal) => {
    if (!request.sessionID) {
      const entry: Entry = { queue: Promise.resolve() }
      return turn(entry, request, signal).finally(() => stop(entry))
    }
    const sessionID = request.sessionID
    const entry = entries.get(sessionID) ?? { queue: Promise.resolve() }
    entries.set(sessionID, entry)
    const next = entry.queue.catch(() => undefined).then(() =>
      turn(entry, request, signal).finally(() => {
        if (entry.idle) clearTimeout(entry.idle)
        entry.idle = setTimeout(() => stop(entry), IDLE_MS)
        entry.idle.unref?.()
      }),
    )
    entry.queue = next
    return next
  }

  return {
    run,
    remove: (sessionID) => {
      const entry = entries.get(sessionID)
      entries.delete(sessionID)
      if (entry) stop(entry)
    },
    close: () => {
      for (const entry of entries.values()) stop(entry)
      entries.clear()
    },
  }
}

function record(value: unknown) {
  return AcpRpc.isRecord(value) ? value : {}
}

// Agents list models either the older way (`models`, set with session/set_model) or as a "model" config option.
function listModels(response: Record<string, unknown>) {
  const models = AcpRpc.isRecord(response.models) ? response.models : undefined
  if (models && Array.isArray(models.availableModels)) {
    return {
      control: { type: "models" } as ModelControl,
      current: typeof models.currentModelId === "string" ? models.currentModelId : undefined,
      models: models.availableModels.filter(AcpRpc.isRecord).flatMap((item) =>
        typeof item.modelId === "string"
          ? [{ id: item.modelId, name: typeof item.name === "string" ? item.name : item.modelId }]
          : [],
      ),
    }
  }
  const options = Array.isArray(response.configOptions) ? response.configOptions.filter(AcpRpc.isRecord) : []
  const option = options.find((item) => item.category === "model" && typeof item.id === "string")
  if (!option || typeof option.id !== "string") return { control: { type: "none" } as ModelControl, models: [] }
  const values = Array.isArray(option.options) ? option.options.filter(AcpRpc.isRecord) : []
  return {
    control: { type: "config", configID: option.id } as ModelControl,
    current: typeof option.currentValue === "string" ? option.currentValue : undefined,
    models: values.flatMap((item) =>
      typeof item.value === "string"
        ? [{ id: item.value, name: typeof item.name === "string" ? item.name : item.value }]
        : [],
    ),
  }
}
