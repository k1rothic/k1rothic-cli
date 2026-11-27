# k1rothic

k1rothic is a fork of [opencode](https://github.com/sst/opencode) (MIT; see
`LICENSE`). It keeps opencode's agent, tools, sessions and providers, and
changes the parts below. Every change is marked with a `k1rothic:` comment in
the code; search for it before merging upstream.

The Go prototype that came before this fork is on the `go-prototype` branch.

## Hard rules

- **No telemetry or phone-home.** Nothing is sent anywhere except to the model
  provider you configure.
- **API keys only in the OS keychain.** Keys live in macOS Keychain, Windows
  Credential Manager or libsecret, never in plaintext files. The database keeps
  only metadata and a blanked copy.
- **Mode A uses your own API keys.** Vendor subscription logins (ChatGPT/Codex,
  GitHub Copilot, SuperGrok, Poe) are disabled.
- **Mode B (`/official`) only starts the official CLI.** It never reads, copies,
  proxies or refreshes vendor tokens or credential files.

## Changes from opencode

| Area | Change | Where |
|---|---|---|
| Name | Binary `k1rothic`; data and config under `~/.local/share/k1rothic`, `~/.config/k1rothic`; config files `k1rothic.json(c)` (`opencode.json(c)` and project `.opencode/` still read); system dir `%ProgramData%\k1rothic` / `/etc/k1rothic` | `util/src/global.ts`, `core/src/config/discovery.ts`, `cli/script/build.ts`, `cli/src/commands/commands.ts` |
| Text | User-visible "OpenCode" strings renamed | `script/k1rothic/rebrand-strings.ts` (re-run after merges) |
| Logo | "k1rothic" wordmark | `tui/src/logo.ts` |
| Themes | carbon (default), graphite, ocean, forest, amber, rose, violet, mono | `script/k1rothic/gen-themes.ts` → `tui/src/theme/assets/*.json`, `tui/src/theme/v1.ts`, `tui/src/context/theme.tsx` |
| Keys | Secrets in the OS keychain via `Bun.secrets` (service `k1rothic`); DB rows redacted; old plaintext rows moved on first read; in-memory store under `bun test` | `core/src/credential/vault.ts`, `core/src/credential.ts` |
| Providers | Removed ChatGPT/Codex login, GitHub Copilot, opencode Console/Zen (free models) and the Claude Agent bridge (drives the local `claude`); SuperGrok and Poe logins off | `core/src/plugin/provider.ts`, `plugin/provider/{openai,xai,poe}.ts` |
| Updates | No update feed (opencode.ai serves opencode builds): background check and `upgrade` disabled | `cli/src/services/updater.ts` |
| Model catalog | Bundled snapshot only; fetch from models.opencode.ai only with `K1ROTHIC_MODELS_FETCH=1` or `OPENCODE_MODELS_URL` | `cli/src/server-process.ts` |
| Pairing / push | `/pair`, `/update` hidden; `k1rothic pair` refuses; push relays empty | `tui/src/app.tsx`, `cli/src/commands/handlers/pair.ts`, `core/src/push/list.ts` |
| MCP OAuth | opencode's hosted client metadata document not used (dynamic registration instead) | `core/src/mcp/oauth.ts` |
| Vendor files | `~/.claude` (CLAUDE.md, skills) not read — project `.claude/` still is; Claude Code IDE lock files (`~/.claude/ide`, contain an auth token) not read | `core/src/config/discovery.ts`, `core/src/config/plugin/instruction.ts`, `tui/src/editor.ts` |
| `/provider` | `/connect` renamed (`/connect` stays as an alias); dialog titled "Providers" | `tui/src/app.tsx`, `component/dialog-integration.tsx` |
| Provider list | Default allowlist: anthropic, openai, openrouter, google, xai, deepseek, mistral, groq, ollama, lmstudio + the user's custom providers; a user `enabled_providers` / `provider.use` policy replaces it | `core/src/k1rothic-providers.ts`, `core/src/config/plugin/policy.ts`, `server/src/handlers/integration.ts` |
| Custom gateways | `/provider` → "+ Add custom gateway": name → API format (OpenAI-compatible, OpenAI Responses, Anthropic-compatible) → base URL → key (OS keychain) → models (typed or loaded from `/models`); saved as a custom provider in the global `k1rothic.jsonc`; "Edit or delete gateways" | `core/src/k1rothic-gateways.ts`, `tui/src/component/dialog-gateway.tsx` |
| Session UI | Top bar (working directory · context meter `██░░ 22.5%  113K / 500K`); user message as a shaded block `❯ text` with the time; `◆ Thought for 2.0s`; status row `⠋ Responding… 15s … 17s ↓9.45k [stop]` above the input; rounded input box with `model (effort) · agent · ask/auto-approve` in its bottom border; footer `Shift+Tab:mode │ Esc:cancel │ Ctrl+P:commands`; session tab row hidden until `alt+t` or `/tabs`; model label drawn lighter (`titleColor`) | `tui/src/component/k1-chrome.tsx`, `component/prompt/index.tsx`, `feature-plugins/prompt/footer.tsx`, `routes/session/{index,group-view,message-parts}.tsx`, `app.tsx`, `util/session.ts` |
| Mode B | `/official` lists claude / codex / agy, finds them on PATH, shows the official install link if missing, shows a one-time subscription-terms notice, then suspends the TUI and runs the CLI with inherited stdio, no arguments and no added env | `tui/src/official.ts`, `tui/src/component/dialog-official.tsx` |

## Build and run

```sh
bun install
bun run dev                                   # run from source
cd packages/cli && bun run script/build.ts --single --skip-web-ui
./dist/cli-<os>-<arch>/bin/k1rothic           # release binary for this machine
```

Use a modern terminal (Windows Terminal, not the legacy console window).

## Merging upstream

```sh
git fetch upstream dev
git merge upstream/dev
bun script/k1rothic/rebrand-strings.ts        # rename new user-visible strings
grep -rn "k1rothic:" packages                 # check every change survived
```
