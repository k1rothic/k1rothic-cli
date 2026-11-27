import { define, type Context } from "@opencode/plugin/effect/plugin"
import { SessionID } from "@opencode/schema/session-id"
import { Effect, Option, Schema, Stream } from "effect"
import { Environment } from "../../../environment/index.js"
import { Model } from "../../../model.js"
import { Provider } from "../../../provider.js"
import type { PluginInternal } from "../../internal.js"
import { ProviderRefresh } from "../refresh.js"
import { AcpAgents } from "./agents.js"
import { AcpLanguageModel } from "./language-model.js"
import { AcpPermission } from "./permission.js"
import { AcpSessions } from "./sessions.js"

// k1rothic: Antigravity (`agy acp`) and Codex (`codex-acp`) as providers. Each chat keeps one conversation in the
// user's own signed-in CLI, running in the background, so the vendor's prompt cache carries over between turns.
// k1rothic shows the CLI's text, thinking and tool activity, and asks the user before the agent's tools run.

const PACKAGE = "@k1rothic/acp-agent"
const METHOD_ID = "cli-login"
const MODELS_KEY = "models"
const sessionKey = (sessionID: string) => `sessions/${sessionID}`

const CatalogModel = Schema.Struct({ id: Schema.String, name: Schema.String })
const Catalog = Schema.Array(CatalogModel)

export const make = (definition: AcpAgents.Definition) =>
  define({
    id: `k1rothic.provider.${definition.providerID}`,
    effect: Effect.fn("AcpAgent.Plugin")(function* (ctx) {
      // The CLI is a local program, so skip it where k1rothic cannot start local processes.
      const environment = yield* Effect.serviceOption(Environment.Service)
      if (Option.isSome(environment) && !environment.value.local) return

      const providerID = Provider.ID.make(definition.providerID)
      const context = yield* Effect.context<never>()
      const run = Effect.runPromiseWith(context)
      const directory = ctx.location.directory
      const saved = Schema.decodeUnknownOption(Catalog)(yield* ctx.storage.get(MODELS_KEY))
      const catalog = { current: Option.getOrElse(saved, () => definition.models) }

      const sessions = AcpSessions.make({
        definition,
        directory,
        load: (sessionID) =>
          run(ctx.storage.get(sessionKey(sessionID))).then((value) =>
            typeof value === "string" ? value : undefined,
          ),
        save: (sessionID, acpSessionID) => run(ctx.storage.set(sessionKey(sessionID), acpSessionID)),
        permission: (input) =>
          run(
            ctx.permission
              .assert({
                sessionID: SessionID.make(input.sessionID),
                ...AcpPermission.describe(input.toolCall, directory),
              })
              .pipe(
                Effect.as(true),
                // Blocked by a rule, or declined with feedback: the agent hears no and goes on.
                Effect.catch(() => Effect.succeed(false)),
              ),
            { signal: input.signal },
          ),
        models: (listed) => {
          const next = [
            ...definition.models.filter((model) => model.id === "default"),
            ...listed.filter((model) => model.id !== "default"),
          ]
          if (JSON.stringify(next) === JSON.stringify(catalog.current)) return
          catalog.current = next
          void run(
            ctx.storage.set(MODELS_KEY, next).pipe(Effect.andThen(ctx.provider.reload()), Effect.ignore),
          )
        },
      })
      yield* Effect.addFinalizer(() => Effect.sync(() => sessions.close()))

      yield* ctx.provider.transform((editor) => {
        editor.add({
          info: { id: providerID, name: definition.name, activation: "auto", package: Provider.aisdk(PACKAGE) },
          models: catalog.current.map((model) => toModel(providerID, model)),
        })
      })

      yield* ctx.integration.transform((editor) => {
        editor.update(definition.providerID, (integration) => {
          integration.name = definition.name
        })
        editor.method.update({
          integrationID: definition.providerID,
          method: { id: METHOD_ID, type: "external", label: definition.integrationLabel },
        })
      })
      const check = reportInstalled(ctx, definition)
      yield* check
      yield* ProviderRefresh.onSwitch(definition.providerID, check)

      yield* ctx.aisdk.hook(
        "sdk",
        (event) =>
          Effect.sync(() => {
            if (event.package !== PACKAGE) return
            event.sdk = {
              languageModel: (modelID: string) =>
                new AcpLanguageModel.Model(definition.providerID, modelID, toolPrefix(definition), sessions),
            }
          }),
        { providerID },
      )

      yield* ctx.session.hook(
        "model.request",
        (event) =>
          Effect.sync(() => {
            event.headers[AcpLanguageModel.KIND_HEADER] = event.kind
            event.headers[AcpLanguageModel.SESSION_HEADER] = event.sessionID
          }),
        { providerID },
      )
      // The CLI already retries its own API failures; replaying the whole turn would run its tools again.
      yield* ctx.session.hook(
        "retry",
        (event) =>
          Effect.sync(() => {
            event.decision = { retry: false }
          }),
        { providerID },
      )
      // A title from the first message, instead of starting another CLI conversation just to name the chat.
      yield* ctx.session.hook(
        "title",
        (event) =>
          Effect.sync(() => {
            const first = event.messages
              .flatMap((message) => (message.role === "user" ? message.content : []))
              .flatMap((part) => (part.type === "text" ? [part.text] : []))
              .join(" ")
              .replace(/\s+/g, " ")
              .trim()
            if (first) event.result = first.length > 60 ? `${first.slice(0, 57)}…` : first
          }),
        { providerID },
      )
      // The CLI keeps and compacts its own conversation; k1rothic's copy is only for display.
      yield* ctx.session.hook(
        "compaction",
        (event) =>
          Effect.sync(() => {
            event.result = {
              summary: `${definition.name} keeps this conversation's history itself; earlier messages are still available to it.`,
            }
          }),
        { providerID },
      )

      yield* ctx.event.subscribe({ types: ["session.deleted"] }).pipe(
        Stream.runForEach((event) =>
          Effect.gen(function* () {
            if (event.type !== "session.deleted") return
            sessions.remove(event.data.sessionID)
            yield* ctx.storage.remove(sessionKey(event.data.sessionID))
          }),
        ),
        Effect.forkScoped({ startImmediately: true }),
      )
    }),
  } satisfies PluginInternal.InternalPlugin)

function toModel(providerID: Provider.ID, model: { readonly id: string; readonly name: string }): Model.Info {
  const defaults = Model.Info.default(providerID, Model.ID.make(model.id))
  return {
    ...defaults,
    name: model.name,
    // The CLI manages its own context window; 0 output means unknown.
    limit: { context: 1_000_000, output: 0 },
    capabilities: { tools: true, input: ["text", "image"], output: ["text"] },
  }
}

function toolPrefix(definition: AcpAgents.Definition) {
  return definition.providerID.replace(/-agent$/, "")
}

const reportInstalled = (ctx: Context, definition: AcpAgents.Definition) =>
  Effect.gen(function* () {
    const connection = yield* ctx.integration.connection.active(definition.providerID)
    if (!connection) return
    const installed = definition.command() !== undefined
    yield* ctx.integration.connection.status({
      integrationID: definition.providerID,
      connection,
      status: installed
        ? undefined
        : {
            status: "needs_auth",
            message: `${definition.name} is not installed. Install it (/official → Install), sign in, then try again.`,
            url: definition.installURL,
          },
    })
  })

export const AntigravityAgentPlugin = make(AcpAgents.antigravity)
export const CodexAgentPlugin = make(AcpAgents.codex)
