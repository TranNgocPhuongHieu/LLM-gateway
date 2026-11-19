// Ready-to-paste client configurations for the onboarding "copy config"
// buttons. They mirror the Claude Code and Codex snippets in the docs.

export type ClientConfigKind = 'curl' | 'claude-code' | 'codex'

// gatewayOrigin turns a chat-completions endpoint back into the API origin.
export function gatewayOrigin(endpoint: string): string {
  return endpoint.replace(/\/+$/, '').replace(/\/v1(\/chat\/completions)?$/, '')
}

export function buildCurlCommand(args: {
  endpoint: string
  apiKey: string
  model: string
}): string {
  return [
    `curl ${args.endpoint} \\`,
    '  -H "Content-Type: application/json" \\',
    `  -H "Authorization: Bearer ${args.apiKey}" \\`,
    `  -d '{"model":"${args.model}","messages":[{"role":"user","content":"Say hello in one sentence."}]}'`,
  ].join('\n')
}

export function buildClaudeCodeConfig(origin: string, apiKey: string): string {
  return [
    `export ANTHROPIC_BASE_URL='${origin}'`,
    `export ANTHROPIC_AUTH_TOKEN='${apiKey}'`,
    'claude',
  ].join('\n')
}

export function buildCodexConfig(origin: string, apiKey: string): string {
  return [
    '# ~/.codex/config.toml',
    'model = "gpt-6.1-sol"',
    'model_provider = "llm"',
    '',
    '[model_providers.llm]',
    'name = "LLM"',
    `base_url = "${origin}/v1"`,
    'env_key = "LLM_API_KEY"',
    'wire_api = "responses"',
    '',
    '# Shell (do not commit your key)',
    `export LLM_API_KEY='${apiKey}'`,
  ].join('\n')
}

export function buildClientConfig(
  kind: ClientConfigKind,
  args: { endpoint: string; apiKey: string; model: string }
): string {
  const origin = gatewayOrigin(args.endpoint)
  switch (kind) {
    case 'claude-code':
      return buildClaudeCodeConfig(origin, args.apiKey)
    case 'codex':
      return buildCodexConfig(origin, args.apiKey)
    default:
      return buildCurlCommand(args)
  }
}
