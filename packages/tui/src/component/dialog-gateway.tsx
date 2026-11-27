// k1rothic: add, edit and delete custom API gateways from /provider.
// A gateway is a custom provider in the user's global config (name, API
// format, base URL, models); its API key goes through the credential store,
// i.e. the OS keychain — never into the config file.
import { createSignal, Show } from "solid-js"
import { Global } from "@opencode/util/global"
import {
  fetchGatewayModels,
  gatewayAPIs,
  gatewayID,
  listGateways,
  removeGateway,
  saveGateway,
  type Gateway,
  type GatewayAPI,
} from "@opencode/core/k1rothic-gateways"
import { DialogSelect } from "../ui/dialog-select"
import { DialogPrompt } from "../ui/dialog-prompt"
import { DialogConfirm } from "../ui/dialog-confirm"
import { useDialog } from "../ui/dialog"
import { useToast } from "../ui/toast"
import { useTheme } from "../context/theme"
import { useData } from "../context/data"
import { useLocation } from "../context/location"
import { useClient } from "../context/client"
import { errorMessage } from "../util/error"
import { DialogModel } from "./dialog-model"

type Draft = Partial<Gateway> & { key?: string; editing?: boolean }

const configDirectory = () => Global.Path.config

function useRefresh() {
  const data = useData()
  const currentLocation = useLocation()
  const location = currentLocation.ref ?? data.location.default()
  // The server reloads config files it watches; wait until the gateway's integration shows up.
  const refresh = async (id?: string) => {
    for (let attempt = 0; attempt < 20; attempt++) {
      data.location.integration.invalidate(location)
      data.location.model.invalidate(location)
      data.location.provider.invalidate(location)
      await Promise.all([
        data.location.integration.sync(location),
        data.location.model.sync(location),
        data.location.provider.sync(location),
      ])
      const present = (data.location.integration.list(location) ?? []).some((item) => item.id === id)
      if (!id || present) return present
      await new Promise((resolve) => setTimeout(resolve, 250))
    }
    return false
  }
  return { refresh, location, data }
}

function ErrorLine(props: { error?: string }) {
  const theme = useTheme().surface("dialog")
  return (
    <Show when={props.error}>{(value) => <text fg={theme.text.feedback.error.base}>{value()}</text>}</Show>
  )
}

/** Step 1: name. */
export function DialogGatewayName() {
  const dialog = useDialog()
  return (
    <DialogPrompt
      title="New custom gateway"
      subtitle="Any server speaking the OpenAI or Anthropic API"
      placeholder="Name, e.g. My LiteLLM"
      onConfirm={(value) => {
        const name = value.trim()
        if (!name) return
        dialog.replace(() => <DialogGatewayAPI draft={{ name }} />)
      }}
    />
  )
}

/** Step 2: API format. */
function DialogGatewayAPI(props: { draft: Draft }) {
  const dialog = useDialog()
  return (
    <DialogSelect
      title={`API format · ${props.draft.name}`}
      current={props.draft.api}
      options={gatewayAPIs.map((api) => ({ value: api.id, title: api.label, description: api.hint }))}
      onSelect={(option) => {
        const draft = { ...props.draft, api: option.value as GatewayAPI }
        if (draft.editing) return void dialog.replace(() => <DialogGatewaySave draft={draft} />)
        dialog.replace(() => <DialogGatewayURL draft={draft} />)
      }}
    />
  )
}

/** Step 3: base URL. */
function DialogGatewayURL(props: { draft: Draft }) {
  const dialog = useDialog()
  const [error, setError] = createSignal<string>()
  const api = gatewayAPIs.find((item) => item.id === props.draft.api)
  return (
    <DialogPrompt
      title={`Base URL · ${props.draft.name}`}
      subtitle={`For example ${api?.example ?? "https://api.example.com/v1"}`}
      placeholder={api?.example ?? "https://"}
      value={props.draft.baseURL}
      onConfirm={(value) => {
        const baseURL = value.trim()
        if (!/^https?:\/\/\S+$/.test(baseURL)) return void setError("The URL must start with http:// or https://")
        const draft = { ...props.draft, baseURL }
        if (draft.editing) return void dialog.replace(() => <DialogGatewaySave draft={draft} />)
        dialog.replace(() => <DialogGatewayKey draft={draft} />)
      }}
      description={() => <ErrorLine error={error()} />}
    />
  )
}

/** Step 4: API key (optional; stored in the OS keychain). */
function DialogGatewayKey(props: { draft: Draft }) {
  const dialog = useDialog()
  return (
    <DialogPrompt
      title={`API key · ${props.draft.name}`}
      subtitle="Stored only in the OS keychain. Leave empty if the server needs no key."
      placeholder="API key (optional)"
      onConfirm={(value) => dialog.replace(() => <DialogGatewayModels draft={{ ...props.draft, key: value.trim() }} />)}
    />
  )
}

/** Step 5: models (typed, or fetched from the server). */
function DialogGatewayModels(props: { draft: Draft }) {
  const dialog = useDialog()
  const [busy, setBusy] = createSignal(false)
  const [error, setError] = createSignal<string>()
  return (
    <DialogPrompt
      title={`Models · ${props.draft.name}`}
      subtitle="Model IDs separated by commas — or leave empty to load the list from the server"
      placeholder="e.g. gpt-4.1, llama3.3"
      value={props.draft.models?.join(", ")}
      busy={busy()}
      busyText="Loading models…"
      onConfirm={(value) => {
        const typed = value
          .split(/[,\s]+/)
          .map((item) => item.trim())
          .filter((item) => item !== "")
        if (typed.length) return void dialog.replace(() => <DialogGatewaySave draft={{ ...props.draft, models: typed }} />)
        setBusy(true)
        setError(undefined)
        void fetchGatewayModels(props.draft.api!, props.draft.baseURL!, props.draft.key)
          .then((models) => {
            if (!models.length) throw new Error("The server listed no models; type the model IDs instead.")
            dialog.replace(() => <DialogGatewaySave draft={{ ...props.draft, models }} />)
          })
          .catch((cause) => {
            setBusy(false)
            setError(`Could not load models: ${errorMessage(cause)}. Type the model IDs instead.`)
          })
      }}
      description={() => <ErrorLine error={error()} />}
    />
  )
}

/** Saves the gateway, stores its key, then opens the model picker for it. */
function DialogGatewaySave(props: { draft: Draft }) {
  const dialog = useDialog()
  const toast = useToast()
  const client = useClient()
  const { refresh, location } = useRefresh()
  const [error, setError] = createSignal<string>()
  void (async () => {
    try {
      const existing = await listGateways(configDirectory())
      const id = props.draft.id ?? gatewayID(props.draft.name!, new Set(existing.map((item) => item.id)))
      const gateway: Gateway = {
        id,
        name: props.draft.name!,
        api: props.draft.api!,
        baseURL: props.draft.baseURL!,
        models: props.draft.models ?? [],
      }
      await saveGateway(configDirectory(), gateway)
      const present = await refresh(id)
      if (!present) throw new Error("The server did not pick up the new gateway; check that k1rothic runs locally.")
      if (props.draft.key)
        await client.api.integration.connect.key({
          integrationID: id,
          location: { directory: location.directory },
          key: props.draft.key,
        })
      await refresh()
      toast.show({ variant: "success", message: `${props.draft.editing ? "Updated" : "Added"} ${gateway.name}` })
      dialog.replace(() => <DialogModel integrationID={id} />)
    } catch (cause) {
      setError(errorMessage(cause))
    }
  })()
  return (
    <DialogPrompt
      title={`Saving ${props.draft.name}…`}
      busy={!error()}
      busyText="Saving the gateway and loading its models…"
      description={() => <ErrorLine error={error()} />}
    />
  )
}

/** The list of custom gateways with add / edit / delete. */
export function DialogGateways() {
  const dialog = useDialog()
  const [gateways, setGateways] = createSignal<Gateway[]>([])
  void listGateways(configDirectory()).then(setGateways)
  return (
    <DialogSelect
      title="Custom gateways"
      options={[
        ...gateways().map((gateway) => ({
          value: gateway.id,
          title: gateway.name,
          description: `${gatewayAPIs.find((api) => api.id === gateway.api)?.label ?? gateway.api} · ${gateway.baseURL}`,
          category: "Gateways",
        })),
        { value: "+add", title: "+ Add custom gateway", description: "OpenAI- or Anthropic-compatible", category: "New" },
      ]}
      onSelect={(option) => {
        if (option.value === "+add") return void dialog.replace(() => <DialogGatewayName />)
        const gateway = gateways().find((item) => item.id === option.value)
        if (gateway) dialog.replace(() => <DialogGatewayMenu gateway={gateway} />)
      }}
    />
  )
}

function DialogGatewayMenu(props: { gateway: Gateway }) {
  const dialog = useDialog()
  const draft: Draft = { ...props.gateway, editing: true }
  return (
    <DialogSelect
      title={props.gateway.name}
      options={[
        { value: "models", title: "Choose a model", description: `${props.gateway.models.length} models` },
        { value: "url", title: "Change base URL", description: props.gateway.baseURL },
        { value: "api", title: "Change API format", description: gatewayAPIs.find((a) => a.id === props.gateway.api)?.label },
        { value: "list", title: "Change model list", description: props.gateway.models.slice(0, 4).join(", ") },
        { value: "key", title: "Set or replace API key", description: "stored in the OS keychain" },
        { value: "delete", title: "Delete gateway", description: "removes it and its saved key" },
      ]}
      onSelect={(option) => {
        switch (option.value) {
          case "models":
            return void dialog.replace(() => <DialogModel integrationID={props.gateway.id} />)
          case "url":
            return void dialog.replace(() => <DialogGatewayURL draft={draft} />)
          case "api":
            return void dialog.replace(() => <DialogGatewayAPI draft={draft} />)
          case "list":
            return void dialog.replace(() => <DialogGatewayModels draft={draft} />)
          case "key":
            return void dialog.replace(() => <DialogGatewayReplaceKey gateway={props.gateway} />)
          case "delete":
            return void dialog.replace(() => <DialogGatewayDelete gateway={props.gateway} />)
        }
      }}
    />
  )
}

function DialogGatewayReplaceKey(props: { gateway: Gateway }) {
  const dialog = useDialog()
  const toast = useToast()
  const client = useClient()
  const { refresh, location } = useRefresh()
  const [error, setError] = createSignal<string>()
  return (
    <DialogPrompt
      title={`API key · ${props.gateway.name}`}
      subtitle="Stored only in the OS keychain"
      placeholder="API key"
      onConfirm={(value) => {
        const key = value.trim()
        if (!key) return
        void client.api.integration.connect
          .key({ integrationID: props.gateway.id, location: { directory: location.directory }, key })
          .then(() => refresh())
          .then(() => {
            toast.show({ variant: "success", message: `Key saved for ${props.gateway.name}` })
            dialog.clear()
          })
          .catch((cause) => setError(errorMessage(cause)))
      }}
      description={() => <ErrorLine error={error()} />}
    />
  )
}

function DialogGatewayDelete(props: { gateway: Gateway }) {
  const dialog = useDialog()
  const toast = useToast()
  const client = useClient()
  const { refresh, location, data } = useRefresh()
  return (
    <DialogConfirm
      title={`Delete ${props.gateway.name}?`}
      message="This removes the gateway from your config and its API key from the OS keychain."
      label={{ confirm: "Delete", cancel: "Keep" }}
      onCancel={() => dialog.replace(() => <DialogGatewayMenu gateway={props.gateway} />)}
      onConfirm={() => {
        void (async () => {
          try {
            const integration = (data.location.integration.list(location) ?? []).find(
              (item) => item.id === props.gateway.id,
            )
            for (const connection of integration?.connections ?? [])
              if (connection.type === "credential") await client.api.credential.remove({ credentialID: connection.id })
            await removeGateway(configDirectory(), props.gateway.id)
            await refresh()
            toast.show({ variant: "success", message: `Deleted ${props.gateway.name}` })
          } catch (cause) {
            toast.error(cause)
          }
        })()
      }}
    />
  )
}
