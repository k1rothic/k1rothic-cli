// k1rothic session chrome: the top bar (working directory · context meter)
// and the streaming status row ("⠋ Responding… 15s   17s ↓9.45k [stop]").
import { createMemo, createSignal, For, onCleanup, Show } from "solid-js"
import { useTheme } from "../context/theme"
import { useData } from "../context/data"
import { useTuiPaths } from "../context/runtime"
import { abbreviateHome } from "../runtime"
import { contextUsage } from "../util/session"

import { sym } from "../k1-glyphs"
/** 950 → "950", 9_450 → "9.5K", 500_000 → "500K", 1_200_000 → "1.2M". */
export function compactTokens(n: number) {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${trim(n / 1000)}K`
  return `${trim(n / 1_000_000)}M`
}

/** Like compactTokens with one more digit for the live output counter: 9450 → "9.45k". */
function fineTokens(n: number) {
  if (n < 1000) return String(n)
  if (n < 100_000) return `${(n / 1000).toFixed(2).replace(/\.?0+$/, "")}k`
  return `${Math.round(n / 1000)}k`
}

function trim(v: number) {
  return v >= 100 ? String(Math.round(v)) : v.toFixed(1).replace(/\.0$/, "")
}

/** 400ms → "0.4s", 15s → "15s", 125s → "2m05s". */
export function elapsed(ms: number) {
  const s = Math.max(0, ms) / 1000
  if (s < 10) return `${s.toFixed(1)}s`
  if (s < 60) return `${Math.floor(s)}s`
  const m = Math.floor(s / 60)
  return `${m}m${String(Math.floor(s % 60)).padStart(2, "0")}s`
}

/** Re-renders every `ms` while mounted. */
function useNow(ms: number) {
  const [now, setNow] = createSignal(Date.now())
  const timer = setInterval(() => setNow(Date.now()), ms)
  onCleanup(() => clearInterval(timer))
  return now
}

export function K1TopBar(props: { sessionID: string }) {
  const theme = useTheme()
  const data = useData()
  const paths = useTuiPaths()
  const session = createMemo(() => data.session.get(props.sessionID))
  const cwd = createMemo(() => {
    const location = session()?.location
    if (!location) return ""
    const directory = abbreviateHome(location.directory, paths.home)
    const branch = data.location.vcs.info(location)?.branch.current
    return branch ? `${directory}:${branch}` : directory
  })
  const usage = createMemo(() => {
    const current = session()
    if (!current) return
    return contextUsage(
      data.session.message.list(props.sessionID),
      data.location.model.list(current.location),
      current.revert?.messageID,
    )
  })
  const meter = createMemo(() => {
    const u = usage()
    const used = u?.tokens ?? 0
    const limit = u?.limit
    if (!limit) return { bar: "", filled: 0, pct: "", text: `${compactTokens(used)} tokens` }
    const frac = Math.min(used / limit, 1)
    const width = 10
    const filled = used > 0 ? Math.max(1, Math.round(frac * width)) : 0
    return {
      bar: "█".repeat(filled) + "░".repeat(width - filled),
      filled,
      frac,
      pct: `${(frac * 100).toFixed(1)}%`,
      text: `${compactTokens(used)} / ${compactTokens(limit)}`,
    }
  })
  const fill = () => {
    const f = meter().frac ?? 0
    if (f >= 0.85) return theme.text.feedback.error.base
    if (f >= 0.6) return theme.text.feedback.warning.base
    return theme.text.action.primary.selected
  }
  return (
    <box flexDirection="row" justifyContent="space-between" flexShrink={0} height={1} gap={2}>
      <text fg={theme.text.muted} wrapMode="none" truncate flexShrink={1}>
        {cwd()}
      </text>
      <text wrapMode="none" flexShrink={0}>
        <Show when={meter().bar}>
          <span style={{ fg: fill() }}>{meter().bar.slice(0, meter().filled)}</span>
          <span style={{ fg: theme.border.base }}>{meter().bar.slice(meter().filled)}</span>
          <span style={{ fg: theme.text.base }}> {meter().pct} </span>
        </Show>
        <span style={{ fg: theme.text.base }}> {meter().text}</span>
      </text>
    </box>
  )
}

const brailleFrames = [sym("⠋"), sym("⠙"), sym("⠹"), sym("⠸"), sym("⠼"), sym("⠴"), sym("⠦"), sym("⠧"), sym("⠇"), sym("⠏")]

export function K1Status(props: { sessionID: string; onStop: () => void }) {
  const theme = useTheme()
  const data = useData()
  const now = useNow(100)
  const turn = createMemo(() => {
    const messages = data.session.message.list(props.sessionID)
    const userIndex = messages.findLastIndex((m) => m.type === "user")
    const user = messages[userIndex]
    const assistant = messages.slice(userIndex + 1).findLast((m) => m.type === "assistant")
    const start = user && "time" in user ? (user.time as { created: number }).created : undefined
    const tokens = assistant && "tokens" in assistant ? (assistant.tokens as { output: number; reasoning: number }) : undefined
    const phaseStart =
      assistant && "time" in assistant ? (assistant.time as { created: number }).created : (start ?? Date.now())
    return { start: start ?? Date.now(), phaseStart, output: (tokens?.output ?? 0) + (tokens?.reasoning ?? 0), assistant }
  })
  const verb = () => (turn().assistant ? "Responding…" : "Thinking…")
  const frame = () => brailleFrames[Math.floor(now() / 80) % brailleFrames.length]
  return (
    <box flexDirection="row" justifyContent="space-between" flexShrink={0} height={1} gap={2}>
      <text wrapMode="none" truncate flexShrink={1}>
        <span style={{ fg: theme.text.action.primary.selected }}> {frame()} </span>
        <For each={Array.from(verb())}>
          {(char, index) => {
            // A three-letter highlight sweeps across the word, like a shimmer.
            const lit = () => {
              const span = verb().length + 6
              const pos = (Math.floor(now() / 70) % span) - 3
              return index() >= pos && index() < pos + 3
            }
            return (
              <span style={{ fg: lit() ? theme.text.base : theme.text.muted, attributes: lit() ? 1 : 0 }}>{char}</span>
            )
          }}
        </For>
        <span style={{ fg: theme.text.muted }}> {elapsed(now() - turn().phaseStart)}</span>
      </text>
      <box flexDirection="row" flexShrink={0} gap={1}>
        <text fg={theme.text.muted} wrapMode="none">
          {elapsed(now() - turn().start)}
          {turn().output > 0 ? ` ↓${fineTokens(turn().output)}` : ""}
        </text>
        <text
          fg={theme.text.muted}
          wrapMode="none"
          onMouseUp={(event: { button: number; stopPropagation(): void }) => {
            if (event.button !== 0) return
            event.stopPropagation()
            props.onStop()
          }}
        >
          [stop]
        </text>
      </box>
    </box>
  )
}
