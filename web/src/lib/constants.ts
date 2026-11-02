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
 * Application-wide constants
 */

// System Configuration Defaults
export const DEFAULT_SYSTEM_NAME = 'AI giá rẻ'
export const LOGO_CONTENT_HASH = '740e706f'
export const FAVICON_CONTENT_HASH = '1af31875'
export const DEFAULT_LOGO = `/logo.png?v=${LOGO_CONTENT_HASH}`
export const DEFAULT_FAVICON = `/favicon.ico?v=${FAVICON_CONTENT_HASH}`

/**
 * Ensures asset URLs like /logo.png and /favicon.ico receive cache-busting version params.
 */
export function getBustedAssetUrl(url?: string): string {
  if (!url) return ''
  const trimmed = url.trim()
  if (trimmed === '/logo.png' || trimmed === 'logo.png') {
    return `/logo.png?v=${LOGO_CONTENT_HASH}`
  }
  if (trimmed === '/favicon.ico' || trimmed === 'favicon.ico') {
    return `/favicon.ico?v=${FAVICON_CONTENT_HASH}`
  }
  return trimmed
}

// LocalStorage Keys
export const STORAGE_KEYS = {
  SYSTEM_NAME: 'system_name',
  LOGO: 'logo',
  FOOTER_HTML: 'footer_html',
} as const
