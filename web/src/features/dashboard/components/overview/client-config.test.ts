import { describe, expect, it } from 'vitest'

import { buildClientConfig, gatewayOrigin } from './client-config'

const args = {
  endpoint: 'https://gw.test/v1/chat/completions',
  apiKey: 'sk-abc',
  model: 'gpt-6.1-sol',
}

describe('client config', () => {
  it('derives the API origin from the chat endpoint', () => {
    expect(gatewayOrigin('https://gw.test/v1/chat/completions')).toBe('https://gw.test')
    expect(gatewayOrigin('https://gw.test/v1/')).toBe('https://gw.test')
    expect(gatewayOrigin('https://gw.test')).toBe('https://gw.test')
  })

  it('builds Claude Code env with the gateway origin and key', () => {
    const config = buildClientConfig('claude-code', args)
    expect(config).toContain("export ANTHROPIC_BASE_URL='https://gw.test'")
    expect(config).toContain("export ANTHROPIC_AUTH_TOKEN='sk-abc'")
  })

  it('builds Codex config against the /v1 responses API', () => {
    const config = buildClientConfig('codex', args)
    expect(config).toContain('base_url = "https://gw.test/v1"')
    expect(config).toContain('wire_api = "responses"')
    expect(config).toContain("export LLM_API_KEY='sk-abc'")
  })

  it('keeps the curl request on the chat endpoint', () => {
    const config = buildClientConfig('curl', args)
    expect(config).toContain('curl https://gw.test/v1/chat/completions')
    expect(config).toContain('Authorization: Bearer sk-abc')
  })
})
