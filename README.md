# k1rothic-cli (working name)

A cross-platform terminal app that combines:

- **Mode A (native):** its own LLM chat and coding agent, using your API keys
  (Anthropic, OpenAI, Gemini, or any endpoint speaking one of their APIs).
- **Mode B (official CLI host):** a launcher that hands the terminal to the
  unmodified official vendor CLIs (`claude`, `codex`, `agy`) so you can use
  them with your subscriptions. k1rothic never touches their credentials.

> Status: **M2** — real streaming chat with Anthropic, OpenAI, Gemini and
> custom endpoints (Chat Completions, Responses, Anthropic Messages or Gemini
> API). Tools and the agent loop arrive in M3.

## Providers and API keys

Run `/provider` (or open settings with **F2**) and pick a provider. If it has
no key yet, k1rothic asks for one and stores it **only in the OS keychain**
(macOS Keychain, Windows Credential Manager, Secret Service on Linux) — never
in a file. `Tab` in the provider list replaces a key; `/provider remove-key`
deletes the current one.

No keychain (e.g. a headless Linux box)? Export
`K1ROTHIC_<PROVIDER>_API_KEY` (`K1ROTHIC_ANTHROPIC_API_KEY`,
`K1ROTHIC_OPENAI_API_KEY`, `K1ROTHIC_GEMINI_API_KEY`,
`K1ROTHIC_CUSTOM_<NAME>_API_KEY` for a custom endpoint) for the session; it is never written
anywhere.

| Provider | API | Reasoning effort maps to |
|---|---|---|
| `anthropic` | Messages API (`anthropic-version: 2026-10-21`) | adaptive thinking + `output_config.effort` |
| `openai` | Chat Completions (default) or Responses — `/provider api` | `reasoning_effort` / `reasoning.effort` + summaries (`off` omits it — use for non-reasoning models) |
| `gemini` | `streamGenerateContent` | `thinkingBudget` (2.x) / `thinkingLevel` (newer) |
| `custom:<name>` | any number of your own endpoints, each with its own API format — see below | depends on the API |
| `fake` | offline scripted demo | — |

### Custom endpoints

Add as many as you like: `/provider` → **+ Add custom endpoint** (or
`/endpoints`). Each asks for a **name**, the **API format**, the base URL and an
optional key:

| API format | Path added to the base URL | Typical servers | Reasoning |
|---|---|---|---|
| OpenAI Chat Completions | `/chat/completions` | Ollama, vLLM, LM Studio, llama.cpp, OpenRouter, Groq, DeepSeek | not sent; `reasoning_content` is shown |
| OpenAI Responses | `/responses` | vLLM, OpenRouter, Azure OpenAI | `reasoning.effort` + summaries |
| Anthropic Messages | `/v1/messages` | DeepSeek, Kimi, MiniMax, GLM, OpenRouter, LiteLLM | `thinking` budget (2K / 8K / 16K) |
| Google Gemini | `/v1beta/models/…:streamGenerateContent` | Gemini API proxies | as for `gemini` |

`/endpoints` (also in F2 settings and via `Tab` in the provider list) edits an
endpoint: use it, change its API format or URL, set/remove its key, or delete
it (asks for confirmation; deleting also removes its key from the keychain).
`/provider <name>` switches to an endpoint directly; `/provider api` and
`/provider url` edit the current one. The single custom server of older
versions is migrated to an endpoint named `custom`.

`/model` lists the provider's models live (type to filter, or type any id and
press Enter); `/model <id>` sets one directly. The last model is remembered
per provider.

## Build

Requires Go 1.26+.

```sh
go build -o k1rothic ./cmd/k1rothic
./k1rothic
```

Static cross-builds: `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build ./cmd/k1rothic`.

## Keys

| Key | Action |
|---|---|
| Enter | send |
| Shift+Enter · Alt+Enter · Ctrl+J | newline (Shift+Enter needs a terminal with keyboard enhancements) |
| Shift+Tab | cycle approval mode (ask → auto-edit → plan) |
| Ctrl+C · Esc | stop the response; Ctrl+C clears input; twice exits |
| Ctrl+O · click | open/close every "Thought for…" block and long prompt (click one to toggle just it) |
| Mouse click | menu items, settings rows, input cursor position, footer shortcuts, `[stop]` |
| PgUp/PgDn · Shift+↑/↓ · mouse wheel | scroll |
| F2 · `/settings` | settings |
| `/theme` | color theme picker with live preview (`/theme ocean` sets it directly) |
| Ctrl+. · F1 | shortcuts |
| `/` | slash commands |

## Config

`<os.UserConfigDir>/k1rothic/config.toml` (override with `K1ROTHIC_CONFIG_DIR`
or `--config`). Everything here can also be changed in the settings panel
(F2), which saves to this file. API keys are never stored in it.

```toml
provider = "anthropic"
model = "claude-opus-5-5"
effort = "medium"  # off, low, medium, high

[approval]
mode = "ask"
allow_always_approve = false

[ui]
theme = "auto"   # background: auto, dark, light
color = "carbon"    # carbon, graphite, ocean, forest, amber, rose, violet, mono
spinner = "braille" # braille, dots, pulse, line (ASCII, if braille shows as boxes)
glyphs = "auto"  # auto, unicode, safe — "safe" avoids symbols many fonts lack (auto = safe on Windows)
mouse = true     # wheel scrolling; hold Shift to select text in most terminals

[endpoints.local]  # use with provider = "custom:local"
api = "chat-completions"  # chat-completions, responses, anthropic-messages, gemini
base_url = "http://localhost:11434/v1"
model = "llama3.3"

[endpoints.deepseek]
api = "anthropic-messages"
base_url = "https://api.deepseek.com/anthropic"
```

No telemetry.
