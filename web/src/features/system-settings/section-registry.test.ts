import { describe, expect, it } from 'vitest'

import { CONTENT_SECTION_IDS, getContentSectionMeta } from './content/section-registry.tsx'
import { MODELS_SECTION_IDS } from './models/section-registry.tsx'
import { OPERATIONS_SECTION_IDS } from './operations/section-registry.tsx'
import { SITE_SECTION_IDS } from './site/section-registry.tsx'

describe('System Notice section placement', () => {
  it('lists notice under Console Content, right before announcements', () => {
    const ids = CONTENT_SECTION_IDS as readonly string[]
    expect(ids).toContain('notice')
    expect(ids.indexOf('notice')).toBe(ids.indexOf('announcements') - 1)
  })

  it('no longer lists notice under Site & Branding', () => {
    expect(SITE_SECTION_IDS as readonly string[]).not.toContain('notice')
  })

  it('uses the System Notice title for the content section', () => {
    expect(getContentSectionMeta('notice').titleKey).toBe('System Notice')
  })
})

describe('legacy settings targets exist in the registries', () => {
  it('models global and operations performance targets are registered sections', () => {
    expect(MODELS_SECTION_IDS as readonly string[]).toContain('global')
    expect(OPERATIONS_SECTION_IDS as readonly string[]).toContain('performance')
  })
})
