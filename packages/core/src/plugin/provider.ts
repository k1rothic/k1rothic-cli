import { AntigravityAgentPlugin, CodexAgentPlugin } from "./provider/acp-agent/index.js"
import { AmazonBedrockPlugin } from "./provider/amazon-bedrock.js"
import { AzurePlugin } from "./provider/azure.js"
import { CerebrasPlugin } from "./provider/cerebras.js"
import { ClaudeAgentPlugin } from "./provider/claude-agent/index.js"
import { CloudflareAIGatewayPlugin } from "./provider/cloudflare-ai-gateway.js"
import { CloudflareWorkersAIPlugin } from "./provider/cloudflare-workers-ai.js"
import { DatabricksPlugin } from "./provider/databricks.js"
import { DigitalOceanPlugin } from "./provider/digitalocean.js"
import { GitLabPlugin } from "./provider/gitlab.js"
import { GoogleVertexPlugin } from "./provider/google-vertex.js"
import { KiloPlugin } from "./provider/kilo.js"
import { LLMGatewayPlugin } from "./provider/llmgateway.js"
import { LMStudioPlugin } from "./provider/lmstudio.js"
import { ModalPlugin } from "./provider/modal.js"
import { NeonPlugin } from "./provider/neon.js"
import { NvidiaPlugin } from "./provider/nvidia.js"
import { OllamaPlugin } from "./provider/ollama.js"
import { OpenAIPlugin } from "./provider/openai.js"
import { OpenAIContextCapsPlugin } from "./provider/openai-context-caps.js"
import { SnowflakeCortexPlugin } from "./provider/snowflake-cortex.js"
import { OpenRouterPlugin } from "./provider/openrouter.js"
import { PoePlugin } from "./provider/poe.js"
import { PromptCacheKeyPlugin } from "./provider/prompt-cache-key.js"
import { SapAICorePlugin } from "./provider/sap-ai-core.js"
import { VLLMPlugin } from "./provider/vllm.js"
import { XAIPlugin } from "./provider/xai.js"
import { ZenmuxPlugin } from "./provider/zenmux.js"
import type { PluginInternal } from "./internal.js"

// k1rothic: removed ChatGPTPlugin (ChatGPT/Codex login), GithubCopilotPlugin (Copilot login) and
// OpencodePlugin (opencode Console/Zen cloud, free models). ClaudeAgentPlugin is back (owner-approved,
// Nov 2026): it drives the user's own signed-in `claude` CLI in the background and never sees a token.
// AntigravityAgentPlugin and CodexAgentPlugin do the same for `agy` and `codex` through their ACP servers.
export const ProviderPlugins: PluginInternal.InternalPlugin[] = [
  AntigravityAgentPlugin,
  CodexAgentPlugin,
  AmazonBedrockPlugin,
  AzurePlugin,
  CerebrasPlugin,
  ClaudeAgentPlugin,
  CloudflareAIGatewayPlugin,
  CloudflareWorkersAIPlugin,
  DatabricksPlugin,
  DigitalOceanPlugin,
  GitLabPlugin,
  GoogleVertexPlugin,
  KiloPlugin,
  LLMGatewayPlugin,
  LMStudioPlugin,
  ModalPlugin,
  NeonPlugin,
  NvidiaPlugin,
  OllamaPlugin,
  SnowflakeCortexPlugin,
  OpenAIPlugin,
  OpenAIContextCapsPlugin,
  OpenRouterPlugin,
  PoePlugin,
  PromptCacheKeyPlugin,
  SapAICorePlugin,
  VLLMPlugin,
  XAIPlugin,
  ZenmuxPlugin,
]
