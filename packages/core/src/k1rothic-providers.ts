// k1rothic: the providers offered by default. Everything else from the
// models.dev catalog is hidden unless the user lists providers themselves
// (`enabled_providers` or `provider.use` policies) or defines a custom
// provider/gateway under `providers` in their config.
export const DEFAULT_PROVIDERS: ReadonlySet<string> = new Set([
  "anthropic",
  "openai",
  "openrouter",
  "google", // Gemini API
  "xai",
  "deepseek",
  "mistral",
  "groq",
  "ollama", // local
  "lmstudio", // local
])

/** Whether a provider stays visible under the default allowlist. */
export function allowedByDefault(id: string, configured: ReadonlySet<string>) {
  return DEFAULT_PROVIDERS.has(id) || configured.has(id)
}
