export * as AcpPrompt from "./prompt.js"

import type { LanguageModelV3FilePart, LanguageModelV3Prompt, LanguageModelV3TextPart } from "@ai-sdk/provider"

// ACP content blocks sent with session/prompt.
export type Block =
  | { readonly type: "text"; readonly text: string }
  | { readonly type: "image"; readonly mimeType: string; readonly data: string }

type Message = LanguageModelV3Prompt[number]

/**
 * The agent keeps the conversation itself, so a turn only sends the user input since the last reply. A new agent
 * conversation in a chat that already has history (a resume that failed, or a chat started on another model) gets
 * the earlier messages as a transcript first.
 */
export function latest(prompt: LanguageModelV3Prompt, fresh: boolean): Block[] {
  const start = trailingUserStart(prompt)
  const input = prompt.slice(start).flatMap((message) => (message.role === "user" ? message.content.flatMap(block) : []))
  const blocks: Block[] = input.length > 0 ? input : [{ type: "text", text: "Continue." }]
  if (!fresh) return blocks
  const history = transcript(prompt.slice(0, start))
  if (!history) return blocks
  return [
    {
      type: "text",
      text: `The conversation so far took place in k1rothic before you joined it:\n\n${history}\n\n---\n\nContinue from here.`,
    },
    ...blocks,
  ]
}

/** A side call (title, summary) as one self-contained message, including k1rothic's instructions for it. */
export function standalone(prompt: LanguageModelV3Prompt): Block[] {
  const system = prompt
    .flatMap((message) => (message.role === "system" ? [message.content] : []))
    .join("\n\n")
    .trim()
  const conversation = transcript(prompt.filter((message) => message.role !== "system"))
  const text = [
    system ? `Instructions:\n${system}` : "",
    conversation,
    "Answer with the requested text only. Do not use any tools.",
  ]
    .filter(Boolean)
    .join("\n\n")
  return [{ type: "text", text }]
}

function trailingUserStart(prompt: LanguageModelV3Prompt) {
  const index = prompt.findLastIndex((message) => message.role !== "user")
  return index + 1
}

function block(part: LanguageModelV3TextPart | LanguageModelV3FilePart): Block[] {
  if (part.type === "text") return part.text ? [{ type: "text", text: part.text }] : []
  if (part.mediaType.startsWith("image/") && !(part.data instanceof URL)) {
    return [{ type: "image", mimeType: part.mediaType, data: base64(part.data) }]
  }
  if (part.mediaType.startsWith("text/") && !(part.data instanceof URL)) {
    const text = typeof part.data === "string" ? Buffer.from(part.data, "base64").toString("utf8") : decode(part.data)
    return [{ type: "text", text: `${part.filename ?? "Attached file"}:\n${text}` }]
  }
  const name = part.filename ?? (part.data instanceof URL ? part.data.toString() : "file")
  return [{ type: "text", text: `[Attached ${part.mediaType}: ${name} (not sent to the agent)]` }]
}

function transcript(messages: readonly Message[]) {
  return messages
    .flatMap((message) => {
      if (message.role === "system" || message.role === "tool") return []
      const text = message.content
        .flatMap((part) => (part.type === "text" && part.text.trim() ? [part.text.trim()] : []))
        .join("\n")
      if (!text) return []
      return [`${message.role === "user" ? "User" : "Assistant"}: ${text}`]
    })
    .join("\n\n")
}

function base64(data: string | Uint8Array) {
  return typeof data === "string" ? data : Buffer.from(data).toString("base64")
}

function decode(data: Uint8Array) {
  return new TextDecoder().decode(data)
}
