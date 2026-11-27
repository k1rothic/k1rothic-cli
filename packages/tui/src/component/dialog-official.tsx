// k1rothic Mode B: pick an official vendor CLI and hand the terminal to it.
import { createMemo } from "solid-js"
import { useRenderer } from "@opentui/solid"
import { DialogSelect } from "../ui/dialog-select"
import { DialogAlert } from "../ui/dialog-alert"
import { DialogConfirm } from "../ui/dialog-confirm"
import { useDialog } from "../ui/dialog"
import { useToast } from "../ui/toast"
import { useStorage } from "../context/storage"
import { launch, locate, termsNotice, vendors, type Vendor } from "../official"
import { errorMessage } from "../util/error"

export function DialogOfficial(props: { cwd?: string }) {
  const dialog = useDialog()
  const toast = useToast()
  const renderer = useRenderer()
  const [state, setState] = useStorage().store("official", { initial: { termsSeen: false } })

  const options = createMemo(() =>
    vendors.map((vendor) => {
      const path = locate(vendor)
      return {
        value: vendor.id,
        title: `${vendor.name} (${vendor.binary})`,
        description: path ? path : `not installed · ${vendor.install}`,
      }
    }),
  )

  const run = (vendor: Vendor, executable: string) => {
    dialog.clear()
    void launch({ renderer, executable, cwd: props.cwd })
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

  const choose = (vendor: Vendor) => {
    const executable = locate(vendor)
    if (!executable) {
      dialog.replace(() => (
        <DialogAlert
          title={`${vendor.name} is not installed`}
          message={`k1rothic does not install vendor tools. Install ${vendor.binary} from the official page, then run /official again: ${vendor.install}`}
        />
      ))
      return
    }
    if (state.termsSeen) return run(vendor, executable)
    dialog.replace(() => (
      <DialogConfirm
        title="Official CLIs use your own subscription"
        message={termsNotice}
        label={{ confirm: `Start ${vendor.binary}`, cancel: "Cancel" }}
        onConfirm={() => {
          void setState((draft) => {
            draft.termsSeen = true
          })
          run(vendor, executable)
        }}
      />
    ))
  }

  return (
    <DialogSelect
      title="Official CLIs"
      options={options()}
      onSelect={(option) => {
        const vendor = vendors.find((v) => v.id === option.value)
        if (vendor) choose(vendor)
      }}
    />
  )
}
