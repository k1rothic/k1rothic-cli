# k1rothic — handoff for the next AI

This file is a briefing for an AI agent continuing work on this repository.
Read it fully before changing anything. `K1ROTHIC.md` has the short list of
differences from opencode; this file has the context, rules, state and open work.

## What k1rothic is

A cross-platform terminal app (Windows is the owner's main platform) with two modes:

- **Mode A (native):** its own chat and coding agent that uses the user's own API keys
  (Anthropic, OpenAI, OpenRouter, Gemini, … and custom gateways).
- **Mode B (host):** a launcher for the unmodified official vendor CLIs
  (`claude`, `codex`, `agy`), used with the user's own subscriptions.

The owner writes in Russian (sometimes English) and sends screenshots of what they
want. Answer in the language they use.

## Hard rules (never break; ask the owner before touching any of these)

1. **No telemetry or phone-home.** The app talks only to the model provider the user
   configured. (Dev tooling note: `turbo` has its own telemetry; run it with
   `TURBO_TELEMETRY_DISABLED=1`.)
2. **API keys only in the OS keychain**, never in plaintext files. The database keeps
   metadata plus a redacted copy (`packages/core/src/credential/vault.ts`).
3. **Mode A uses API keys only.** No third-party use of vendor subscription OAuth
   (Claude Pro/Max, ChatGPT/Codex login, Copilot, SuperGrok, Poe).
4. **Mode B absolute rules:**
   - Start the vendor CLI as a normal interactive process: no `-p`/`--print`/`exec`/
     `--json`/`stream-json`, no SDKs, no ACP/MCP bridges, no wrapping it as a backend.
   - Never parse its output to drive our logic (stdio is inherited).
   - Never read, copy, store, proxy or refresh its OAuth tokens or credential files
     (`~/.claude*`, `~/.codex*`, Antigravity/Gemini dirs, keychain entries); never
     inject tokens via env. The vendor CLI logs in by itself.
   - Locate binaries on PATH; if missing, show the official install link; never auto-install.
   - First-run notice: subscription terms are set by each vendor and can change.
5. Ask before adding dependencies with restrictive licenses. Prefer boring, maintained deps.
6. Ask before credential-related changes or any change to the rules above.
7. Commit or push only when the owner asks.

## Repository history and state

- Branch **`go-prototype`** (commit `b970f28`): the first version, written in Go with
  Bubble Tea v2. Kept for reference only; it is not developed further.
- Branch **`main`**: a fork of **sst/opencode** (MIT, Bun + TypeScript, OpenTUI + Solid).
  It is based on upstream commit `b7a7135` (remote `upstream` → `https://github.com/sst/opencode.git`,
  branch `dev`, fetched with `--depth 1`).
- **All k1rothic work on `main` is uncommitted** (about 120 changed/new files). The
  owner has not yet asked for a commit. Every change is marked with a `k1rothic:`
  comment in the code; `grep -rn "k1rothic:" packages` finds them.

## Environment notes (owner's Windows machine)

- Bun **1.4.4** installed globally via `npm i -g bun@1.4.4` (opencode pins it).
- Python is cp1251 by default: use `PYTHONIOENCODING=utf-8` for scripts printing Unicode.
- In the Bash tool, heredocs mangle backslashes/escapes: write scripts to files first.
- The owner often runs a k1rothic build; `k1rothic.exe` plus its background service
  (`serve --service`) lock the build folder. Do not kill the owner's processes; build into
  a new `--outdir` instead (`dist`, `dist-ui`, `dist-next` already exist and are git-ignored).
- The owner's Go-version keys are still in Windows Credential Manager as
  `k1rothic:custom` and `k1rothic:custom:openrouter` (go-keyring format). The opencode
  fork does not read them; the owner re-enters keys via `/provider`. Don't delete them.

## Build, run, test

```sh
bun install
bun run dev                                    # from repo root: runs packages/cli/src/index.ts
cd packages/cli && bun run script/build.ts --single --skip-web-ui --skip-install --outdir=dist-next
./dist-next/cli-windows-x64/bin/k1rothic.exe   # release binary for this machine
```

- Run from `packages/cli` (or with `--cwd packages/cli`): its `bunfig.toml` loads the
  OpenTUI JSX transform. Running from elsewhere compiles JSX as React and draws nothing.
- `--standalone` uses a private server instead of the background service.
- `OPENCODE_SIMULATE=1` blocks real network calls (useful for UI checks without a key).
- Type checks: `TURBO_TELEMETRY_DISABLED=1 bun turbo typecheck --filter=@opencode/tui --filter=@opencode/core --filter=@opencode/cli --filter=@opencode/server`
  (or `bun check --no-script -b` inside a package; core uses `-p tsconfig.tests.json`).
- Tests: `bun test <files>` inside a package. Under `bun test` (NODE_ENV=test) the
  keychain is an in-memory store, so tests never touch the real keychain.
- Screenshots / UI automation: opencode's **Drive mode** (`OPENCODE_DRIVE=<name>`,
  `OPENCODE_DRIVE_RENDERER=headless`, manifest in `DRIVE_REGISTRY_DIR/<name>.json`) exposes
  a JSON-RPC WebSocket (`simulation.handshake`, `ui.type`, `ui.enter`, `ui.press`,
  `ui.capture` → colored span lines). A working driver script was drafted but not finished;
  it must spawn the app with `cwd = packages/cli`.

## What has been changed (summary; details in K1ROTHIC.md)

| Area | Change | Main files |
|---|---|---|
| Branding | Name/binary `k1rothic`; data/config in `~/.local/share/k1rothic`, `~/.config/k1rothic`; `k1rothic.json(c)` (opencode.json(c) still read); visible "OpenCode" strings renamed; "k1rothic" logo | `util/src/global.ts`, `core/src/config/discovery.ts`, `cli/script/build.ts`, `cli/src/commands/commands.ts`, `tui/src/logo.ts`, `script/k1rothic/rebrand-strings.ts` |
| Themes | carbon (default), graphite, ocean, forest, amber, rose, violet, mono | `script/k1rothic/gen-themes.ts`, `tui/src/theme/assets/*.json`, `tui/src/theme/v1.ts`, `tui/src/context/theme.tsx` |
| Keys | OS keychain via `Bun.secrets` (service `k1rothic`, chunked for Windows' 2.5 KB limit); DB rows redacted; old plaintext rows migrated on read | `core/src/credential/vault.ts`, `core/src/credential.ts` |
| Removed | ChatGPT/Codex, Copilot, opencode Console/Zen, Claude Agent bridge, SuperGrok/Poe logins; update feed; models.dev fetch (bundled snapshot; opt in with `K1ROTHIC_MODELS_FETCH=1`); `/pair`, push relays; opencode MCP client metadata doc | `core/src/plugin/provider.ts`, `plugin/provider/{openai,xai,poe}.ts`, `cli/src/services/updater.ts`, `cli/src/server-process.ts`, `core/src/push/list.ts`, `core/src/mcp/oauth.ts`, `cli/src/commands/handlers/pair.ts` |
| Vendor files | `~/.claude` (CLAUDE.md, skills) not read; Claude Code IDE lock files (`~/.claude/ide`, contain a token) not read | `core/src/config/discovery.ts`, `core/src/config/plugin/instruction.ts`, `tui/src/editor.ts` |
| Mode B | `/official`: claude/codex/agy on PATH, install link if missing, one-time terms notice, suspend TUI → spawn with inherited stdio, no args, no added env | `tui/src/official.ts`, `tui/src/component/dialog-official.tsx` |
| Session UI | Top bar (cwd · context meter `██░░ 22.5%  113K / 500K`); user message as shaded block `❯ text  8:19 AM`; `◆ Thought for 2.0s`; status row `⠋ Responding… 15s … 17s ↓9.45k [stop]`; rounded input box with `model (effort) · agent · ask/auto-approve` in the bottom border (lighter `titleColor`); footer `Shift+Tab:mode │ Esc:cancel │ Ctrl+P:commands` | `tui/src/component/k1-chrome.tsx`, `component/prompt/index.tsx`, `feature-plugins/prompt/footer.tsx`, `routes/session/{index,group-view,message-parts}.tsx`, `util/session.ts` |
| Session tabs | Hidden by default; `alt+t` or `/tabs` toggles them | `tui/src/app.tsx`, `tui/src/config/keybind.ts` |
| `/provider` | `/connect` renamed (alias kept); only anthropic, openai, openrouter, google, xai, deepseek, mistral, groq, ollama, lmstudio + user's custom providers (a user `enabled_providers`/`provider.use` policy replaces the list) | `core/src/k1rothic-providers.ts`, `core/src/config/plugin/policy.ts`, `server/src/handlers/integration.ts`, `tui/src/component/dialog-integration.tsx` |
| Custom gateways | `/provider` → "+ Add custom gateway": name → API format (OpenAI-compatible / OpenAI Responses / Anthropic-compatible) → base URL → key (keychain) → models (typed or fetched); saved as a custom provider in the global `k1rothic.jsonc`; edit/delete | `core/src/k1rothic-gateways.ts`, `tui/src/component/dialog-gateway.tsx` |
| Glyphs | On Windows, symbols missing from common fonts (❯ ✓ ◆ ⚠ braille spinner, rounded corners…) are swapped for WGL4 ones through `sym()`; `K1ROTHIC_GLYPHS=unicode|safe` overrides | `tui/src/k1-glyphs.ts`, `script/k1rothic/wrap-glyphs.py` (re-run after merges) |

## Test status

- New k1rothic tests pass: `core/test/credential-vault.test.ts`, the keychain test in
  `core/test/credential.test.ts`, `core/test/k1rothic-gateways.test.ts`, the two
  `k1rothic:` tests in `core/test/config/policy.test.ts`, `tui/test/official.test.ts`,
  `tui/test/k1-glyphs.test.ts`.
- **Known red tests (to fix next):**
  - TUI: upstream opencode fails 15 tests on this Windows machine; with k1rothic it is
    about 86. The extra ~70 assert the old opencode screen text (e.g. "Shell" in the old
    metadata row, "esc interrupt" hint, "+ Thought", theme "opencode", footer usage text,
    "OpenCode" strings). Behaviour was spot-checked and is fine; the tests need updating to
    the new UI (or `k1rothic:`-marked skips for removed features).
  - Core: 141 of 8064 fail; ~62 also fail on unmodified upstream on Windows (shell,
    ripgrep, symlinks). The rest test removed features (push relays, Poe/ChatGPT/Codex
    logins, MCP client metadata doc, Console/Opencode plugin, `~/.claude` reading,
    system dir name, config file names).

## Open work, in priority order

1. Update/skip the ~70 TUI and the core tests that assert removed or redesigned behaviour.
2. Verify in the owner's terminal: the `/provider` dialog and the custom-gateway flow
   (only type-checked and unit-tested, never clicked through), `/official` with a real
   `claude`, the label colour, `alt+t` tabs toggle.
3. Finish the screenshot driver (Drive mode) if the owner still wants screenshots.
4. Verify the `agy` (Google Antigravity) install and terms links in `tui/src/official.ts`.
5. Assistant message timestamps (the target design shows the time right-aligned on the
   assistant's first line) are not implemented.
6. Release/update story: there is no k1rothic update feed; `upgrade` is disabled.
7. When merging upstream: `git fetch upstream dev && git merge upstream/dev`, then
   `bun script/k1rothic/rebrand-strings.ts`, `PYTHONIOENCODING=utf-8 python script/k1rothic/wrap-glyphs.py`,
   and check every `k1rothic:` change survived.

## Conventions

- Mark every deviation from opencode with a `k1rothic:` comment, keep changes small and
  local (feature flags like `const k1rothicHidden = true as boolean` instead of deleting
  upstream code), so upstream merges stay manageable.
- Keep `K1ROTHIC.md` (difference list) and this file up to date when you change behaviour.
- The owner judges by screenshots: after UI work, run the app (e.g. in a terminal tab with
  `OPENCODE_SIMULATE=1 bun run --cwd packages/cli src/index.ts --standalone --prompt '…'`)
  and look at the result before reporting.
