// k1rothic: credential secrets live only in the OS keychain (macOS Keychain,
// Windows Credential Manager, libsecret on Linux) via Bun.secrets. The
// database row keeps the metadata (integration, label, active flag) and a
// redacted copy of the value, never the secret itself.
import type { Credential } from "../credential.js"

/** Keychain service name every credential is stored under. */
export const SERVICE = "k1rothic"

// Windows Credential Manager caps a secret at 2560 bytes (UTF-16), so long
// OAuth payloads are split into chunks stored as "<id>#<n>".
const CHUNK = 1000
const CHUNKED = "k1-chunks:"

interface Secrets {
  get(options: { service: string; name: string }): Promise<string | null>
  set(options: { service: string; name: string; value: string }): Promise<void>
  delete(options: { service: string; name: string }): Promise<boolean>
}

// Tests must never touch the real keychain: under `bun test` (NODE_ENV=test)
// an in-memory store stands in for it.
const memory = new Map<string, string>()
const memoryStore: Secrets = {
  get: async ({ name }) => memory.get(name) ?? null,
  set: async ({ name, value }) => void memory.set(name, value),
  delete: async ({ name }) => memory.delete(name),
}

const secrets = (): Secrets | undefined =>
  process.env.NODE_ENV === "test" ? memoryStore : (globalThis as { Bun?: { secrets?: Secrets } }).Bun?.secrets

export class Unavailable extends Error {
  constructor(cause?: unknown) {
    super(
      "No OS keychain is available, so the key was not saved (k1rothic never stores keys in files). " +
        "Set the provider's API key environment variable for this session instead." +
        (cause instanceof Error ? ` (${cause.message})` : ""),
    )
    this.name = "KeychainUnavailable"
  }
}

/** Whether a keychain backend exists in this runtime. */
export const available = () => secrets() !== undefined

/** Reads the secret value stored for a credential, or undefined. */
export async function read(id: string): Promise<unknown | undefined> {
  const store = secrets()
  if (!store) return undefined
  const head = await store.get({ service: SERVICE, name: id })
  if (head === null) return undefined
  if (!head.startsWith(CHUNKED)) return JSON.parse(head)
  const count = Number(head.slice(CHUNKED.length))
  const parts: string[] = []
  for (let i = 0; i < count; i++) {
    const part = await store.get({ service: SERVICE, name: `${id}#${i}` })
    if (part === null) return undefined // incomplete write: treat as missing
    parts.push(part)
  }
  return JSON.parse(parts.join(""))
}

/** Stores the secret value for a credential, replacing any previous one. */
export async function write(id: string, value: Credential.Value): Promise<void> {
  const store = secrets()
  if (!store) throw new Unavailable()
  const json = JSON.stringify(value)
  try {
    await remove(id)
    if (json.length <= CHUNK) {
      await store.set({ service: SERVICE, name: id, value: json })
      return
    }
    const count = Math.ceil(json.length / CHUNK)
    for (let i = 0; i < count; i++)
      await store.set({ service: SERVICE, name: `${id}#${i}`, value: json.slice(i * CHUNK, (i + 1) * CHUNK) })
    await store.set({ service: SERVICE, name: id, value: CHUNKED + count })
  } catch (error) {
    throw error instanceof Unavailable ? error : new Unavailable(error)
  }
}

/** Removes a credential's secret (and its chunks) from the keychain. */
export async function remove(id: string): Promise<void> {
  const store = secrets()
  if (!store) return
  const head = await store.get({ service: SERVICE, name: id }).catch(() => null)
  if (head?.startsWith(CHUNKED)) {
    const count = Number(head.slice(CHUNKED.length))
    for (let i = 0; i < count; i++) await store.delete({ service: SERVICE, name: `${id}#${i}` }).catch(() => false)
  }
  await store.delete({ service: SERVICE, name: id }).catch(() => false)
}

/** The value as written to the database: same shape, secrets blanked. */
export function redact(value: Credential.Value): Credential.Value {
  switch (value.type) {
    case "key":
      return { type: "key", key: "" }
    case "oauth":
      return { type: "oauth", methodID: value.methodID, refresh: "", access: "", expires: value.expires }
    default:
      return value // external: no secret is stored here
  }
}

/** Whether a database value still holds a secret (rows from before the keychain). */
export function holdsSecret(value: { type: string; key?: unknown; access?: unknown; refresh?: unknown }) {
  if (value.type === "key") return typeof value.key === "string" && value.key !== ""
  if (value.type === "oauth") return value.access !== "" || value.refresh !== ""
  return false
}
