export * as AcpPermission from "./permission.js"

import path from "node:path"
import { AcpRpc } from "./rpc.js"

/**
 * Describes an agent's tool call in k1rothic's permission vocabulary, so the session's rules and saved approvals
 * ("always allow") apply to the agent's own tools too.
 */
export function describe(toolCall: Record<string, unknown>, directory: string) {
  const title = typeof toolCall.title === "string" ? toolCall.title : "Agent tool"
  const raw = AcpRpc.isRecord(toolCall.rawInput) ? toolCall.rawInput : {}
  const files = locations(toolCall.locations, directory)
  const metadata = { title, kind: toolCall.kind }
  const action = ACTIONS[typeof toolCall.kind === "string" ? toolCall.kind : ""]
  if (action === "edit" || action === "read") {
    const resources = files.length > 0 ? files : [title]
    return { action, resources, save: resources, metadata }
  }
  if (action === "shell") {
    const command =
      text(raw.command) || (Array.isArray(raw.command) ? raw.command.join(" ") : "") || text(raw.CommandLine) || title
    return { action, resources: [command], save: [command], metadata }
  }
  if (action === "webfetch") {
    const url = text(raw.url) || title
    return { action, resources: [url], save: [url], metadata }
  }
  return { action: "agent_tool", resources: [title], save: [title], metadata }
}

const ACTIONS: Record<string, string> = {
  edit: "edit",
  delete: "edit",
  move: "edit",
  read: "read",
  execute: "shell",
  fetch: "webfetch",
}

function locations(value: unknown, directory: string) {
  if (!Array.isArray(value)) return []
  return value.flatMap((item) => {
    if (!AcpRpc.isRecord(item) || typeof item.path !== "string") return []
    const relative = path.relative(directory, path.resolve(directory, item.path))
    const inside = relative && !relative.startsWith("..") && !path.isAbsolute(relative)
    return [(inside ? relative : item.path).split(path.sep).join("/")]
  })
}

function text(value: unknown) {
  return typeof value === "string" ? value : ""
}
