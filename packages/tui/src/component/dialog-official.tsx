// k1rothic Mode B: official vendor CLIs (Claude Code, Codex, Antigravity) on the user's own subscription.
// Install (official installer, after confirmation), sign in (the CLI's own login), and start the CLI
// interactively on a chosen model. See ../official.ts for the rules.
import { createMemo } from "solid-js"
import { useRenderer } from "@opentui/solid"
import { openUrl } from "@opencode/util/open"
import { DialogSelect } from "../ui/dialog-select"
import { DialogConfirm } from "../ui/dialog-confirm"
import { DialogPrompt } from "../ui/dialog-prompt"
import { useDialog } from "../ui/dialog"
import { useToast } from "../ui/toast"
import { useStorage } from "../context/storage"
import {
  cacheNotice,
  install,
  launch,
  launchArgs,
  locate,
  termsNotice,
  validModel,
  vendors,
  type Vendor,
  type VendorID,
} from "../official"
import { errorMessage } from "../util/error"
import { sym } from "../k1-glyphs"

export const OFFICIAL_CATEGORY = "Subscriptions (official CLIs)"

type OfficialState = {
  termsSeen: boolean
  /** The model k1rothic last started each CLI with ("" for the CLI's default). */
  last?: Partial<Record<VendorID, string>>
}

function useOfficial(cwd?: string) {
  const dialog = useDialog()
  const toast = useToast()
  const renderer = useRenderer()
  const [state, setState] = useStorage().store<OfficialState>("official", { initial: { termsSeen: false } })

  const run = (vendor: Vendor, args: readonly string[], model?: string) => {
    void setState((draft) => {
      draft.termsSeen = true
      if (model !== undefined) draft.last = { ...draft.last, [vendor.id]: model }
    })
    dialog.clear()
    const executable = locate(vendor)
    if (!executable) return
    void launch({ renderer, executable, args, cwd })
      .then((code) =>
        toast.show({
          variant: code === 0 || code === null ? "info" : "warning",
          message: `Back in k1rothic — ${vendor.binary} exited${code === null ? "" : ` with code ${code}`}.`,
        }),
      )
      .catch((error) =>
        toast.show({ variant: "error", title: `Could not start ${vendor.binary}`, message: errorMessage(error) }),
      )
  }

  // One confirmation that carries every notice that applies (first-use terms, lost prompt cache).
  const confirmThen = (vendor: Vendor, notices: { title: string; message: string }[], go: () => void) => {
    if (!notices.length) return go()
    dialog.replace(() => (
      <DialogConfirm
        title={notices[0]!.title}
        message={notices.map((notice) => notice.message).join("\n\n")}
        label={{ confirm: `Start ${vendor.binary}`, cancel: "Cancel" }}
        onConfirm={go}
      />
    ))
  }

  const terms = () =>
    state.termsSeen ? [] : [{ title: "Official CLIs use your own subscription", message: termsNotice }]

  return {
    lastModel: (vendor: Vendor) => state.last?.[vendor.id],
    start(vendor: Vendor, input: { model?: string; resume?: boolean }) {
      if (!locate(vendor)) return dialog.replace(() => <DialogOfficialVendor vendor={vendor} cwd={cwd} />)
      const previous = state.last?.[vendor.id]
      const changed = input.resume === true && previous !== undefined && previous !== (input.model ?? "")
      confirmThen(
        vendor,
        [...(changed ? [{ title: "Model changed: the prompt cache is lost", message: cacheNotice }] : []), ...terms()],
        () => run(vendor, launchArgs(vendor, input), input.model ?? ""),
      )
    },
    signIn(vendor: Vendor) {
      confirmThen(vendor, terms(), () => run(vendor, vendor.signIn))
    },
    install(vendor: Vendor, onInstalled?: () => void) {
      const command = process.platform === "win32" ? vendor.installer.win32 : vendor.installer.posix
      dialog.replace(() => (
        <DialogConfirm
          title={`Install ${vendor.name}`}
          message={`k1rothic will run the official installer in this terminal:\n\n  ${command}\n\nSource: ${vendor.install}`}
          label={{ confirm: "Install", cancel: "Cancel" }}
          onConfirm={() => {
            void install({ renderer, vendor })
              .then(() => {
                if (!locate(vendor))
                  return toast.show({
                    variant: "warning",
                    message: `${vendor.binary} was not found after the installer ran. Open a new terminal or check ${vendor.install}`,
                  })
                toast.show({ variant: "success", message: `${vendor.name} installed. Sign in next.` })
                if (onInstalled) return onInstalled()
                dialog.replace(() => <DialogOfficialVendor vendor={vendor} cwd={cwd} />)
              })
              .catch((error) =>
                toast.show({ variant: "error", title: "Installer failed", message: errorMessage(error) }),
              )
          }}
        />
      ))
    },
  }
}

/** The vendors, used by /official. */
export function DialogOfficial(props: { cwd?: string }) {
  const dialog = useDialog()
  const options = createMemo(() =>
    vendors.map((vendor) => {
      const path = locate(vendor)
      return {
        value: vendor.id,
        title: `${vendor.name} (${vendor.binary})`,
        description: path ? `${vendor.plan} · ${path}` : `${vendor.plan} · not installed`,
        onSelect: () => dialog.replace(() => <DialogOfficialVendor vendor={vendor} cwd={props.cwd} />),
      }
    }),
  )
  return <DialogSelect title="Official CLIs" options={options()} />
}

/** Install, sign in, or start one vendor CLI on a chosen model. */
export function DialogOfficialVendor(props: { vendor: Vendor; cwd?: string; onInstalled?: () => void }) {
  const dialog = useDialog()
  const toast = useToast()
  const official = useOfficial(props.cwd)
  const executable = locate(props.vendor)
  const pick = (model?: string) =>
    dialog.replace(() => <DialogOfficialStart vendor={props.vendor} model={model} cwd={props.cwd} />)
  const openInstallPage = () => void openUrl(props.vendor.install).catch(toast.error)

  if (!executable)
    return (
      <DialogSelect
        title={`${props.vendor.name} · not installed`}
        options={[
          {
            value: "install",
            title: `Install ${props.vendor.binary}`,
            description: process.platform === "win32" ? props.vendor.installer.win32 : props.vendor.installer.posix,
            onSelect: () => official.install(props.vendor, props.onInstalled),
          },
          {
            value: "page",
            title: "Open the official install page",
            description: props.vendor.install,
            onSelect: openInstallPage,
          },
        ]}
      />
    )

  const last = official.lastModel(props.vendor)
  return (
    <DialogSelect
      title={`${props.vendor.name} · ${props.vendor.plan}`}
      options={[
        {
          value: "",
          title: "Default model",
          description: "the CLI's own choice (its /model setting)",
          category: "Start with model",
          footer: last === "" ? "last used" : undefined,
          onSelect: () => pick(),
        },
        ...props.vendor.models.map((model) => ({
          value: model.id,
          title: model.title,
          description: model.description ? `${model.id} · ${model.description}` : model.id,
          category: "Start with model",
          footer: last === model.id ? "last used" : undefined,
          onSelect: () => pick(model.id),
        })),
        {
          value: "custom",
          title: "Custom model…",
          description: "type any model id the CLI accepts",
          category: "Start with model",
          onSelect: () =>
            dialog.replace(() => (
              <DialogPrompt
                title={`${props.vendor.name} model`}
                placeholder="Model id"
                value={last && !props.vendor.models.some((model) => model.id === last) ? last : undefined}
                onConfirm={(value) => {
                  const model = value.trim()
                  if (!model) return
                  if (!validModel(model))
                    return toast.show({ variant: "error", message: `Not a valid model name: ${model}` })
                  pick(model)
                }}
              />
            )),
        },
        {
          value: "signin",
          title: "Sign in / switch account",
          description:
            props.vendor.signIn.length > 0
              ? `runs ${props.vendor.binary} ${props.vendor.signIn.join(" ")}`
              : `starts ${props.vendor.binary}; sign in when it asks (or type /login)`,
          category: "Account",
          onSelect: () => official.signIn(props.vendor),
        },
        {
          value: "page",
          title: "Install page",
          description: props.vendor.install,
          category: "Account",
          onSelect: openInstallPage,
        },
      ]}
    />
  )
}

/** New conversation or continue the last one; continuing on another model warns about the prompt cache. */
function DialogOfficialStart(props: { vendor: Vendor; model?: string; cwd?: string }) {
  const official = useOfficial(props.cwd)
  const previous = official.lastModel(props.vendor)
  const changed = previous !== undefined && previous !== (props.model ?? "")
  return (
    <DialogSelect
      title={`${props.vendor.name} · ${props.model ?? "default model"}`}
      options={[
        {
          value: "new",
          title: "New conversation",
          onSelect: () => official.start(props.vendor, { model: props.model }),
        },
        {
          value: "resume",
          title: "Continue last conversation",
          description: changed
            ? `${sym("⚠")} last started with ${previous || "the default model"}: switching drops the prompt cache`
            : undefined,
          onSelect: () => official.start(props.vendor, { model: props.model, resume: true }),
        },
      ]}
    />
  )
}
