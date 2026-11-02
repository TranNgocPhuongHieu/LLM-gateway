/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * LobeHub Icon Loader
 * Dynamically load and render icons from @lobehub/icons
 *
 * Supports:
 * - Basic: "OpenAI", "OpenAI.Color"
 * - Chained properties: "OpenAI.Avatar.type={'platform'}"
 * - Size parameter: getLobeIcon("OpenAI", 20)
 */
import { toc } from '@lobehub/icons/es/toc'
import { useEffect, useRef, useState } from 'react'
import type React from 'react'

import { IconSub2api } from '@/assets/custom/icon-sub2api'

const CUSTOM_ICONS: Record<string, React.ComponentType<{ size?: number }>> = {
  Sub2API: IconSub2api,
}

const iconNames = new Set(toc.map((icon) => icon.id))
const loadedIcons = new Map<string, unknown>()
const pendingIcons = new Map<string, Promise<unknown>>()

function loadIcon(name: string): Promise<unknown> {
  let pending = pendingIcons.get(name)
  if (!pending) {
    // One chunk per brand, rather than retaining the entire dynamic registry.
    pending = import(
      /* webpackInclude: /\/[A-Z][^/]*\/index\.js$/ */
      /* webpackExclude: /\/(components|features|hooks|types)\// */
      `@lobehub/icons/es/${name}/index.js`
    ).then((module: { default: unknown }) => {
      loadedIcons.set(name, module.default)
      return module.default
    })
    pendingIcons.set(name, pending)
    void pending.catch(() => pendingIcons.delete(name))
  }
  return pending
}

function LobeIcon({
  iconName,
  size,
}: {
  iconName?: string | null
  size: number
}) {
  const name = typeof iconName === 'string' ? iconName.trim().split('.')[0] : ''
  const element = useRef<HTMLSpanElement>(null)
  const [loaded, setLoaded] = useState<{ name: string; icon: unknown } | null>(
    null
  )
  const [failed, setFailed] = useState<string | null>(null)
  const icon =
    loadedIcons.get(name) ?? (loaded?.name === name ? loaded.icon : undefined)

  useEffect(() => {
    if (!iconNames.has(name) || CUSTOM_ICONS[name] || loadedIcons.has(name))
      return
    let active = true
    const load = () => {
      void loadIcon(name).then(
        (value) => {
          if (active) setLoaded({ name, icon: value })
        },
        (error: unknown) => {
          if (active) setFailed(name)
          console.warn('Could not load brand icon', error)
        }
      )
    }
    // The selector contains the full catalog: off-screen options must not
    // download every brand as soon as the dropdown opens.
    const observer =
      typeof IntersectionObserver === 'undefined'
        ? null
        : new IntersectionObserver((entries) => {
            if (entries.some((entry) => entry.isIntersecting)) {
              observer?.disconnect()
              load()
            }
          })
    if (observer && element.current) observer.observe(element.current)
    else load()
    return () => {
      active = false
      observer?.disconnect()
    }
  }, [name])

  return (
    <span
      ref={element}
      data-lobe-icon={name}
      data-icon-error={failed === name || undefined}
      className='inline-flex shrink-0 items-center justify-center'
      style={{ width: size, height: size }}
    >
      {renderLobeIcon(iconName, size, icon)}
    </span>
  )
}

/**
 * Parse a property value from string to appropriate type
 * @param raw - Raw string value
 * @returns Parsed value (boolean, number, or string)
 */
function parseValue(raw: string | undefined | null): string | number | boolean {
  if (raw == null) return true

  let v = String(raw).trim()

  // Remove curly braces
  if (v.startsWith('{') && v.endsWith('}')) {
    v = v.slice(1, -1).trim()
  }

  // Remove quotes
  if (
    (v.startsWith('"') && v.endsWith('"')) ||
    (v.startsWith("'") && v.endsWith("'"))
  ) {
    return v.slice(1, -1)
  }

  // Boolean
  if (v === 'true') return true
  if (v === 'false') return false

  // Number
  if (/^-?\d+(?:\.\d+)?$/.test(v)) return Number(v)

  // Return as string
  return v
}

/**
 * Get LobeHub icon component by name
 * @param iconName - Icon name/description (e.g., "OpenAI", "OpenAI.Color", "Claude.Avatar")
 * @param size - Icon size (default: 20)
 * @returns Icon component or fallback
 *
 * @example
 * getLobeIcon("OpenAI", 24)
 * getLobeIcon("OpenAI.Color", 20)
 * getLobeIcon("Claude.Avatar.type={'platform'}", 32)
 */
export function getLobeIcon(
  iconName: string | undefined | null,
  size: number = 20
): React.ReactNode {
  return <LobeIcon iconName={iconName} size={size} />
}

function renderLobeIcon(
  iconName: string | undefined | null,
  size: number,
  loadedIcon: unknown
): React.ReactNode {
  if (!iconName || typeof iconName !== 'string') {
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        ?
      </div>
    )
  }

  const trimmedName = iconName.trim()
  if (!trimmedName) {
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        ?
      </div>
    )
  }

  // Parse component path and chained properties
  const segments = trimmedName.split('.')
  const baseKey = segments[0]
  const CustomIcon = CUSTOM_ICONS[baseKey]
  if (CustomIcon) {
    return <CustomIcon size={size} />
  }
  const BaseIcon = loadedIcon as Record<string, unknown> | undefined

  let IconComponent: React.ComponentType<Record<string, unknown>> | undefined
  let propStartIndex: number

  if (BaseIcon && segments.length > 1 && BaseIcon[segments[1]]) {
    IconComponent = BaseIcon[segments[1]] as React.ComponentType<
      Record<string, unknown>
    >
    propStartIndex = 2
  } else {
    IconComponent = loadedIcon as
      | React.ComponentType<Record<string, unknown>>
      | undefined
    propStartIndex = segments.length > 1 && /^[A-Z]/.test(segments[1]) ? 2 : 1
  }

  // Fallback if icon not found
  if (
    !IconComponent ||
    (typeof IconComponent !== 'function' && typeof IconComponent !== 'object')
  ) {
    const firstLetter = trimmedName.charAt(0).toUpperCase()
    return (
      <div
        className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
        style={{ width: size, height: size }}
      >
        {firstLetter}
      </div>
    )
  }

  // Parse chained properties (e.g., "type={'platform'}", "shape='square'")
  const props: Record<string, string | number | boolean> = {}

  for (let i = propStartIndex; i < segments.length; i++) {
    const seg = segments[i]
    if (!seg) continue

    const eqIdx = seg.indexOf('=')
    if (eqIdx === -1) {
      props[seg.trim()] = true
      continue
    }

    const key = seg.slice(0, eqIdx).trim()
    const valRaw = seg.slice(eqIdx + 1).trim()
    props[key] = parseValue(valRaw)
  }

  // Set size if not explicitly specified in the string
  if (props.size == null && size != null) {
    props.size = size
  }

  return <IconComponent {...props} />
}

// The selector uses the same installed icon registry as the renderer.
export function getLobeIconNames(): string[] {
  const names = toc.flatMap((icon) =>
    icon.param.hasColor ? [icon.id, `${icon.id}.Color`] : [icon.id]
  )
  return [...new Set([...names, ...Object.keys(CUSTOM_ICONS)])].sort()
}
