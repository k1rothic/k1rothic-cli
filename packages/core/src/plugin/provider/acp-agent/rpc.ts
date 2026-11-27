export * as AcpRpc from "./rpc.js"

import type { ChildProcessWithoutNullStreams } from "node:child_process"
import path from "node:path"

// k1rothic: a minimal Agent Client Protocol (JSON-RPC 2.0 over stdio) client for the vendor CLIs' own ACP servers
// (`agy acp`, `codex-acp`). The CLI keeps its own login; k1rothic never sees a token.

export interface Command {
  readonly command: string
  readonly args: readonly string[]
  readonly env?: Readonly<Record<string, string>>
}

export class RpcError extends Error {
  constructor(
    readonly code: number,
    message: string,
    readonly data?: unknown,
  ) {
    super(message)
  }
}

type Handler = (params: Record<string, unknown>) => Promise<unknown>

export interface Connection {
  readonly request: (method: string, params: unknown) => Promise<unknown>
  readonly notify: (method: string, params: unknown) => void
  readonly onNotification: (listener: (method: string, params: Record<string, unknown>) => void) => () => void
  readonly handle: (method: string, handler: Handler) => void
  readonly exited: Promise<string>
  readonly running: () => boolean
  readonly close: () => void
}

export async function start(input: Command & { readonly cwd: string }): Promise<Connection> {
  const { spawn } = await import("node:child_process")
  // npm installs Windows shims as .cmd files, which only start through a shell. The arguments are fixed by k1rothic.
  const shell = process.platform === "win32" && [".cmd", ".bat"].includes(path.extname(input.command).toLowerCase())
  const child: ChildProcessWithoutNullStreams = spawn(input.command, [...input.args], {
    cwd: input.cwd,
    env: { ...process.env, ...input.env },
    stdio: ["pipe", "pipe", "pipe"],
    windowsHide: true,
    shell,
  })
  let id = 0
  let alive = true
  const pending = new Map<number, { resolve: (value: unknown) => void; reject: (error: Error) => void }>()
  const listeners = new Set<(method: string, params: Record<string, unknown>) => void>()
  const handlers = new Map<string, Handler>()
  const stderr: string[] = []

  const write = (message: unknown) => {
    if (!alive) return
    child.stdin.write(JSON.stringify(message) + "\n")
  }

  const exited = new Promise<string>((resolve) => {
    const finish = (reason: string) => {
      if (!alive) return
      alive = false
      const detail = stderr.join("").trim().split("\n").slice(-5).join("\n")
      const error = new Error(detail ? `${reason}: ${detail}` : reason)
      for (const item of pending.values()) item.reject(error)
      pending.clear()
      resolve(error.message)
    }
    child.once("error", (error) => finish(`${input.command} failed to start (${error.message})`))
    child.once("exit", (code) => finish(`${path.basename(input.command)} exited with code ${code}`))
  })

  child.stderr.setEncoding("utf8")
  child.stderr.on("data", (chunk: string) => {
    stderr.push(chunk)
    if (stderr.length > 50) stderr.shift()
  })

  let buffer = ""
  child.stdout.setEncoding("utf8")
  child.stdout.on("data", (chunk: string) => {
    buffer += chunk
    const lines = buffer.split("\n")
    buffer = lines.pop() ?? ""
    for (const line of lines) {
      if (!line.trim()) continue
      const message = parse(line)
      if (message) receive(message)
    }
  })

  const receive = (message: Record<string, unknown>) => {
    const method = typeof message.method === "string" ? message.method : undefined
    const params = isRecord(message.params) ? message.params : {}
    if (method && message.id !== undefined) {
      const handler = handlers.get(method)
      if (!handler) {
        write({ jsonrpc: "2.0", id: message.id, error: { code: -32601, message: `Method not found: ${method}` } })
        return
      }
      handler(params).then(
        (result) => write({ jsonrpc: "2.0", id: message.id, result: result ?? null }),
        (error: unknown) =>
          write({
            jsonrpc: "2.0",
            id: message.id,
            error: { code: -32603, message: error instanceof Error ? error.message : String(error) },
          }),
      )
      return
    }
    if (method) {
      for (const listener of listeners) listener(method, params)
      return
    }
    if (typeof message.id !== "number") return
    const waiting = pending.get(message.id)
    if (!waiting) return
    pending.delete(message.id)
    if (isRecord(message.error)) {
      const error = message.error
      waiting.reject(
        new RpcError(
          typeof error.code === "number" ? error.code : -32603,
          typeof error.message === "string" ? error.message : "ACP request failed",
          error.data,
        ),
      )
      return
    }
    waiting.resolve(message.result)
  }

  return {
    request: (method, params) =>
      new Promise((resolve, reject) => {
        if (!alive) return reject(new Error(`${path.basename(input.command)} is not running`))
        const next = ++id
        pending.set(next, { resolve, reject })
        write({ jsonrpc: "2.0", id: next, method, params })
      }),
    notify: (method, params) => write({ jsonrpc: "2.0", method, params }),
    onNotification: (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    handle: (method, handler) => handlers.set(method, handler),
    exited,
    running: () => alive,
    close: () => {
      if (!alive) return
      child.stdin.end()
      child.kill()
    },
  }
}

function parse(line: string) {
  try {
    const value: unknown = JSON.parse(line)
    return isRecord(value) ? value : undefined
  } catch {
    // Agents may print non-protocol lines on stdout; they are not messages.
    return undefined
  }
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}
