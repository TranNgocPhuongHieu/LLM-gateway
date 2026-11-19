// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'

import { MODELS_SECTION_IDS } from './models/section-registry'
import { OPERATIONS_SECTION_IDS } from './operations/section-registry'
import { SECURITY_SECTION_IDS } from './security/section-registry'

describe('system settings section registries', () => {
  it('keeps the supported model sections', () => {
    expect(MODELS_SECTION_IDS).toEqual([
      'global',
      'routing-reliability',
      'claude',
      'channel-affinity',
    ])
  })

  it('does not register the Gemini and Grok model sections', () => {
    expect(MODELS_SECTION_IDS).not.toContain('gemini')
    expect(MODELS_SECTION_IDS).not.toContain('grok')
  })

  it('does not register the Sensitive Words security section', () => {
    expect(SECURITY_SECTION_IDS).not.toContain('sensitive-words')
    expect(SECURITY_SECTION_IDS).toEqual(
      expect.arrayContaining(['rate-limit', 'ssrf', 'token-limits'])
    )
  })

  it('does not register the Worker Proxy operations section', () => {
    expect(OPERATIONS_SECTION_IDS).not.toContain('worker')
    expect(OPERATIONS_SECTION_IDS).toEqual(
      expect.arrayContaining(['behavior', 'alerts', 'email', 'logs', 'performance'])
    )
  })
})
