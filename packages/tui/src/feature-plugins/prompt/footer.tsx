import { Plugin } from "@opencode/plugin/tui"
import { createMemo, createSignal, For, Match, Show, Switch } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import { TextAttributes } from "@opentui/core"
import { useInteractivity } from "../../context/interactivity"
import { createGoal, GoalStatus } from "./goal"
import { busyCounts, busyTab } from "../../routes/session/composer/tabs"

// k1rothic: "Shift+Tab:mode │ Esc:cancel │ Ctrl+P:commands". Context usage moved to the session top bar.
function prettyKey(value: string) {
  const names: Record<string, string> = { escape: "Esc", return: "Enter" }
  return value
    .split("+")
    .map((part) => names[part] ?? (part.length <= 1 ? part.toUpperCase() : part[0]!.toUpperCase() + part.slice(1)))
    .join("+")
}

export function PromptFooter(props: {
  context: Plugin.Context
  sessionID?: string
  mode: "normal" | "shell"
  showDetails: boolean
}) {
  const dimensions = useTerminalDimensions()
  const [liveHovered, setLiveHovered] = createSignal(false)
  const counts = createMemo(() =>
    props.sessionID
      ? busyCounts({ data: props.context.data, sessionID: props.sessionID, location: props.context.location })
      : { scripts: 0, subagents: 0, shells: 0 },
  )
  const scripts = createMemo(() => label(counts().scripts, "script"))
  const subagents = createMemo(() => label(counts().subagents, "subagent"))
  const shells = createMemo(() => label(counts().shells, "shell"))
  const live = createMemo(() => [scripts(), subagents(), shells()].filter((item): item is string => Boolean(item)))
  const goal = createGoal(props.context, () => props.sessionID)
  // Prompt hints read as disabled while the session does not own input, e.g. while a side panel is focused.
  const interactive = useInteractivity()
  const shortcut = (id: string) => props.context.keymap.shortcuts(id)[0]
  const hints = createMemo(() =>
    [
      ["agent.cycle", "mode"],
      ["session.interrupt", "cancel"],
      ["command.palette.show", "commands"],
    ].flatMap(([id, label]) => {
      const key = shortcut(id!)
      return key ? [{ key: prettyKey(key), label: label! }] : []
    }),
  )
  const hintStyle = () =>
    interactive()
      ? { key: props.context.theme.text.base, label: props.context.theme.text.muted }
      : { key: props.context.theme.text.action.primary.disabled, label: props.context.theme.text.action.primary.disabled }

  return (
    <Switch>
      <Match when={props.mode === "normal"}>
        <box flexDirection="row" flexShrink={1} minWidth={0} gap={2}>
          <Show when={live().length > 0}>
            <box
              flexShrink={0}
              onMouseOver={() => setLiveHovered(true)}
              onMouseOut={() => setLiveHovered(false)}
              onMouseUp={() => props.context.keymap.dispatch("session.child.first", busyTab(counts()))}
            >
              <text fg={liveHovered() ? props.context.theme.text.base : props.context.theme.text.muted} wrapMode="none">
                {live().join(" · ")}
              </text>
            </box>
          </Show>
          <Show when={props.sessionID && goal()}>
            <GoalStatus context={props.context} sessionID={props.sessionID ?? ""} goal={goal} />
          </Show>
          <Show when={props.showDetails && dimensions().width >= 44}>
            <text wrapMode="none" flexShrink={0} attributes={interactive() ? 0 : TextAttributes.DIM}>
              <For each={hints()}>
                {(hint, index) => (
                  <>
                    <Show when={index() > 0}>
                      <span style={{ fg: props.context.theme.border.base }}>{"  │  "}</span>
                    </Show>
                    <span style={{ fg: hintStyle().key }}>{hint.key}</span>
                    <span style={{ fg: hintStyle().label }}>:{hint.label}</span>
                  </>
                )}
              </For>
            </text>
          </Show>
        </box>
      </Match>
      <Match when={props.mode === "shell"}>
        <text fg={props.context.theme.text.base} flexShrink={0}>
          esc{" "}
          <span style={{ fg: props.context.theme.text.muted }}>
            {dimensions().width < 44 ? "shell" : "exit shell mode"}
          </span>
        </text>
      </Match>
    </Switch>
  )
}

export default Plugin.define({
  id: "opencode.prompt.footer",
  setup(context) {
    context.ui.slot({
      append: "prompt.footer",
      render: (props) => (
        <PromptFooter context={context} sessionID={props.sessionID} mode={props.mode} showDetails={props.showDetails} />
      ),
    })
  },
})

function label(count: number, noun: string) {
  return count ? `${count} ${noun}${count === 1 ? "" : "s"}` : undefined
}
