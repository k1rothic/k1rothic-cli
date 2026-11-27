export * as AcpLanguageModel from "./language-model.js"

import type {
  LanguageModelV3,
  LanguageModelV3CallOptions,
  LanguageModelV3StreamPart,
  LanguageModelV3StreamResult,
  LanguageModelV3Usage,
} from "@ai-sdk/provider"
import type { AcpSessions } from "./sessions.js"
import { AcpRpc } from "./rpc.js"

export const KIND_HEADER = "x-k1rothic-acp-kind"
export const SESSION_HEADER = "x-k1rothic-acp-session"

export class Model implements LanguageModelV3 {
  readonly specificationVersion = "v3"
  readonly supportedUrls = {}

  constructor(
    readonly provider: string,
    readonly modelId: string,
    readonly tool: string,
    readonly sessions: AcpSessions.Interface,
  ) {}

  async doGenerate(_options: LanguageModelV3CallOptions): Promise<never> {
    throw new Error("Subscription CLI models only support streaming")
  }

  async doStream(options: LanguageModelV3CallOptions): Promise<LanguageModelV3StreamResult> {
    const abort = new AbortController()
    const signal = options.abortSignal ? AbortSignal.any([abort.signal, options.abortSignal]) : abort.signal
    const headers = options.headers ?? {}
    const sessionID = headers[KIND_HEADER] === "primary" ? headers[SESSION_HEADER] : undefined
    const convert = converter(this.tool)
    let closed = false
    let task: Promise<void> = Promise.resolve()
    const stream = new ReadableStream<LanguageModelV3StreamPart>({
      start: (controller) => {
        controller.enqueue({ type: "stream-start", warnings: [] })
        task = this.sessions
          .run(
            {
              sessionID,
              model: this.modelId,
              prompt: options.prompt,
              emit: (update) => {
                if (closed) return
                for (const part of convert.update(update)) controller.enqueue(part)
              },
            },
            signal,
          )
          .then(
            (result) => {
              if (closed) return
              for (const part of convert.finish(result)) controller.enqueue(part)
              closed = true
              controller.close()
            },
            (error: unknown) => {
              if (closed) return
              closed = true
              controller.error(error)
            },
          )
      },
      cancel: () => {
        closed = true
        abort.abort()
        return task
      },
    })
    return { stream }
  }
}

// Turns ACP session updates into AI SDK stream parts. The agent runs its own tools, so they appear as provider-executed
// calls with their results.
function converter(tool: string) {
  const open: { text?: string; reasoning?: string } = {}
  const calls = new Map<string, { readonly name: string; done: boolean }>()
  let segment = 0

  const close = (): LanguageModelV3StreamPart[] => {
    const parts: LanguageModelV3StreamPart[] = []
    if (open.text) parts.push({ type: "text-end", id: open.text })
    if (open.reasoning) parts.push({ type: "reasoning-end", id: open.reasoning })
    open.text = undefined
    open.reasoning = undefined
    return parts
  }

  const delta = (kind: "text" | "reasoning", text: string): LanguageModelV3StreamPart[] => {
    if (!text) return []
    const parts: LanguageModelV3StreamPart[] = []
    const other = kind === "text" ? "reasoning" : "text"
    if (open[other]) {
      parts.push({ type: `${other}-end`, id: open[other] })
      open[other] = undefined
    }
    const id = open[kind] ?? `${kind}-${segment++}`
    if (!open[kind]) {
      open[kind] = id
      parts.push({ type: `${kind}-start`, id })
    }
    parts.push({ type: `${kind}-delta`, id, delta: text })
    return parts
  }

  const result = (id: string, update: Record<string, unknown>): LanguageModelV3StreamPart[] => {
    const call = calls.get(id)
    if (!call || call.done) return []
    if (update.status !== "completed" && update.status !== "failed") return []
    call.done = true
    const failed = update.status === "failed"
    const output = contentText(update.content) || rawOutput(update.rawOutput) || (failed ? "Failed" : "Done")
    return [
      {
        type: "tool-result",
        toolCallId: id,
        toolName: call.name,
        // Already in k1rothic's tool result shape, so it is stored as plain text rather than as JSON.
        result: { type: failed ? "error" : "text", value: output },
        isError: failed,
      },
    ]
  }

  return {
    update(update: Record<string, unknown>): LanguageModelV3StreamPart[] {
      const kind = update.sessionUpdate
      if (kind === "agent_message_chunk") return delta("text", chunkText(update.content))
      if (kind === "agent_thought_chunk") return delta("reasoning", chunkText(update.content))
      if (kind === "tool_call" && typeof update.toolCallId === "string") {
        if (calls.has(update.toolCallId)) return result(update.toolCallId, update)
        const name = `${tool}_${typeof update.kind === "string" ? update.kind : "tool"}`
        calls.set(update.toolCallId, { name, done: false })
        const input: Record<string, unknown> = { title: typeof update.title === "string" ? update.title : name }
        if (AcpRpc.isRecord(update.rawInput) && Object.keys(update.rawInput).length > 0) input.input = update.rawInput
        return [
          ...close(),
          {
            type: "tool-call",
            toolCallId: update.toolCallId,
            toolName: name,
            input: JSON.stringify(input),
            providerExecuted: true,
          },
          ...result(update.toolCallId, update),
        ]
      }
      if (kind === "tool_call_update" && typeof update.toolCallId === "string") return result(update.toolCallId, update)
      return []
    },
    finish(outcome: AcpSessions.Result): LanguageModelV3StreamPart[] {
      // Calls the agent never finished (a cancelled turn) still need a result.
      const unfinished = [...calls.entries()].flatMap(([id, call]): LanguageModelV3StreamPart[] =>
        call.done
          ? []
          : [
              {
                type: "tool-result",
                toolCallId: id,
                toolName: call.name,
                result: { type: "error", value: "Stopped" },
                isError: true,
              },
            ],
      )
      return [
        ...close(),
        ...unfinished,
        {
          type: "finish",
          finishReason: {
            unified:
              outcome.stopReason === "end_turn"
                ? "stop"
                : outcome.stopReason === "max_tokens" || outcome.stopReason === "max_turn_requests"
                  ? "length"
                  : outcome.stopReason === "refusal"
                    ? "content-filter"
                    : "other",
            raw: outcome.stopReason,
          },
          usage: usage(outcome.usage),
        },
      ]
    },
  }
}

function chunkText(content: unknown) {
  if (!AcpRpc.isRecord(content)) return ""
  return content.type === "text" && typeof content.text === "string" ? content.text : ""
}

function contentText(content: unknown) {
  if (!Array.isArray(content)) return ""
  return content
    .filter(AcpRpc.isRecord)
    .flatMap((item) => {
      if (item.type === "content") {
        const text = chunkText(item.content)
        return text ? [text] : []
      }
      if (item.type === "diff" && typeof item.path === "string") return [`Changed ${item.path}`]
      return []
    })
    .join("\n")
}

function rawOutput(value: unknown) {
  if (value === undefined || value === null) return ""
  if (typeof value === "string") return value
  return JSON.stringify(value)
}

function usage(value: Record<string, unknown> | undefined): LanguageModelV3Usage {
  const number = (key: string) => {
    const item = value?.[key]
    return typeof item === "number" ? item : undefined
  }
  const input = number("inputTokens")
  const read = number("cachedReadTokens")
  const write = number("cachedWriteTokens")
  const output = number("outputTokens")
  const reasoning = number("thoughtTokens")
  const cached = (read ?? 0) + (write ?? 0)
  // Agents differ (agy even between turns) on whether inputTokens includes the cached part. It can't be smaller than
  // the cached part when it does.
  const inclusive = input !== undefined && input >= cached
  return {
    inputTokens: {
      total: input === undefined ? undefined : inclusive ? input : input + cached,
      noCache: input === undefined ? undefined : inclusive ? input - cached : input,
      cacheRead: read,
      cacheWrite: write,
    },
    outputTokens: {
      total: output,
      reasoning,
      text: output === undefined ? undefined : Math.max(0, output - (reasoning ?? 0)),
    },
  }
}
