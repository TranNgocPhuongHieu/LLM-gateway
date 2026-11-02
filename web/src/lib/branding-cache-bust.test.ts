import { describe, it, expect, beforeEach } from 'vitest'
import {
  DEFAULT_LOGO,
  DEFAULT_FAVICON,
  LOGO_CONTENT_HASH,
  FAVICON_CONTENT_HASH,
  getBustedAssetUrl,
} from './constants'
import { applyFaviconToDom } from './dom-utils'

describe('Asset Cache Busting', () => {
  it('defines correct hashes and default asset URLs', () => {
    expect(LOGO_CONTENT_HASH).toBe('740e706f')
    expect(FAVICON_CONTENT_HASH).toBe('1af31875')
    expect(DEFAULT_LOGO).toBe('/logo.png?v=740e706f')
    expect(DEFAULT_FAVICON).toBe('/favicon.ico?v=1af31875')
  })

  it('normalizes bare /logo.png and /favicon.ico', () => {
    expect(getBustedAssetUrl('/logo.png')).toBe('/logo.png?v=740e706f')
    expect(getBustedAssetUrl('logo.png')).toBe('/logo.png?v=740e706f')
    expect(getBustedAssetUrl('/favicon.ico')).toBe('/favicon.ico?v=1af31875')
    expect(getBustedAssetUrl('favicon.ico')).toBe('/favicon.ico?v=1af31875')
  })

  it('preserves already versioned or external URLs', () => {
    expect(getBustedAssetUrl('/logo.png?v=740e706f')).toBe('/logo.png?v=740e706f')
    expect(getBustedAssetUrl('https://example.com/logo.png')).toBe('https://example.com/logo.png')
    expect(getBustedAssetUrl('')).toBe('')
    expect(getBustedAssetUrl(undefined)).toBe('')
  })

  describe('applyFaviconToDom', () => {
    beforeEach(() => {
      document.querySelectorAll('link[rel~="icon"]').forEach((el) => el.remove())
    })

    it('sets cache-busted icon href in DOM', () => {
      applyFaviconToDom('/logo.png')
      const links = document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')
      expect(links.length).toBe(1)
      expect(links[0].href).toContain('/logo.png?v=740e706f')
    })

    it('sets cache-busted favicon href in DOM', () => {
      applyFaviconToDom('/favicon.ico')
      const links = document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')
      expect(links.length).toBe(1)
      expect(links[0].href).toContain('/favicon.ico?v=1af31875')
    })
  })
})
