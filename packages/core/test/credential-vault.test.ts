import { describe, expect, test } from "bun:test"
import * as Vault from "../src/credential/vault"

describe("credential vault (k1rothic)", () => {
  test("round-trips small and chunked values", async () => {
    const key = { type: "key", key: "sk-test" } as const
    await Vault.write("cred_small", key)
    expect(await Vault.read("cred_small")).toEqual(key)

    const big = {
      type: "oauth",
      methodID: "m",
      access: "a".repeat(5000),
      refresh: "r".repeat(3000),
      expires: 1,
    } as never
    await Vault.write("cred_big", big)
    expect(await Vault.read("cred_big")).toEqual(big)

    // Overwriting a chunked value with a small one leaves no stale chunks.
    await Vault.write("cred_big", key)
    expect(await Vault.read("cred_big")).toEqual(key)

    await Vault.remove("cred_big")
    await Vault.remove("cred_small")
    expect(await Vault.read("cred_big")).toBeUndefined()
    expect(await Vault.read("cred_small")).toBeUndefined()
  })

  test("redacts secrets for the database", () => {
    expect(Vault.redact({ type: "key", key: "sk", metadata: { a: 1 } })).toEqual({ type: "key", key: "" })
    const oauth = Vault.redact({ type: "oauth", methodID: "m" as never, access: "x", refresh: "y", expires: 5 })
    expect(oauth as unknown).toEqual({ type: "oauth", methodID: "m", access: "", refresh: "", expires: 5 })
    expect(Vault.holdsSecret({ type: "key", key: "" })).toBe(false)
    expect(Vault.holdsSecret({ type: "key", key: "sk" })).toBe(true)
    expect(Vault.holdsSecret(oauth)).toBe(false)
  })
})
