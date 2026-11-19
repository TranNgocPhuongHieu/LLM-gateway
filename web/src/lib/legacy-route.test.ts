import { describe, expect, it } from 'vitest'

import { resolveLegacyRoute } from './legacy-route'

describe('resolveLegacyRoute', () => {
  it('moves the old site notice path to the content notice section', () => {
    expect(resolveLegacyRoute('https://legacy-route.invalid/system-settings/site/notice')).toBe(
      '/system-settings/content/notice'
    )
  })

  it('keeps query and hash when redirecting the moved notice path', () => {
    expect(
      resolveLegacyRoute('https://legacy-route.invalid/system-settings/site/notice/?x=1#top')
    ).toBe('/system-settings/content/notice?x=1#top')
  })

  it('does not touch other site sections', () => {
    expect(resolveLegacyRoute('https://legacy-route.invalid/system-settings/site/system-info')).toBeNull()
  })
})
