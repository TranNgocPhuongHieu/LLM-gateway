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
import { useNavigate, useRouter } from '@tanstack/react-router'
import { RotateCw, Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { cn } from '@/lib/utils'

type GeneralErrorProps = React.HTMLAttributes<HTMLDivElement> & {
  minimal?: boolean
  error?: unknown
}

export function sanitizeErrorMessage(msg: string): string {
  if (!msg || typeof msg !== 'string') return ''

  return msg
    // 1. Redact URL query parameters: ?api_key=..., &password=..., ?token=..., etc.
    .replace(
      /([?&](?:api[_-]?key|apikey|access[_-]?token|auth[_-]?token|token|secret|password|passwd|pwd)=)[^&\s#"'`]+/gi,
      '$1[REDACTED]'
    )
    // 2. Redact Authorization headers: Authorization: Basic ..., Authorization: Bearer ...
    .replace(
      /(authorization\s*:\s*(?:basic|bearer)\s+)[^\r\n,;\s]+/gi,
      '$1[REDACTED]'
    )
    .replace(
      /(proxy-authorization\s*:\s*(?:basic|bearer)\s+)[^\r\n,;\s]+/gi,
      '$1[REDACTED]'
    )
    // 3. Standalone Basic auth credentials or Bearer tokens: Basic dXNlcjp...
    .replace(/\b(basic\s+)[a-zA-Z0-9+/=]{8,}/gi, '$1[REDACTED]')
    .replace(/\b(bearer\s+)[a-zA-Z0-9_\-\.]{8,}/gi, '$1[REDACTED]')
    // 4. Key-value / form / query string: password=..., password: "...", api_key=...
    .replace(
      /(\b(?:password|passwd|pwd|secret|api[_-]?key|apikey|access[_-]?token|auth[_-]?token)\b\s*[:=]\s*["']?)[^"'\s,;&]+/gi,
      '$1[REDACTED]'
    )
    // 5. JSON string field: "password": "...", "token": "..."
    .replace(
      /("?(?:password|passwd|pwd|secret|api[_-]?key|apikey|access[_-]?token|auth[_-]?token)"?\s*:\s*")[^"]*(")/gi,
      '$1[REDACTED]$2'
    )
    // 6. OpenAI / Anthropic / Provider API keys: sk-...
    .replace(/sk-[a-zA-Z0-9_\-\.]{15,}/gi, '[REDACTED_API_KEY]')
    // 7. JWT tokens: eyJ...
    .replace(/ey[a-zA-Z0-9_\-]{15,}\.[a-zA-Z0-9_\-]{15,}\.[a-zA-Z0-9_\-]{15,}/gi, '[REDACTED_JWT]')
    // 8. Generic bearer / token / secret / password followed by long string
    .replace(
      /\b(?:bearer|token|key|password|secret)\s+[a-zA-Z0-9_\-\.]{8,}/gi,
      '[REDACTED]'
    )
    // 9. URLs with username:password@
    .replace(/(https?:\/\/[^:\/\s]+):([^@\/\s]+)@/gi, '$1:[REDACTED]@')
}

export interface SafeErrorPayload {
  name: string
  message: string
  code?: string
  status?: number
  path?: string
  timestamp: string
}

export function buildSafeErrorPayload(error: unknown, status?: number): SafeErrorPayload {
  const name =
    error instanceof Error
      ? error.name
      : typeof error === 'object' && error !== null && 'name' in error
        ? String((error as { name: unknown }).name)
        : 'Error'

  let code: string | undefined
  if (typeof error === 'object' && error !== null && 'code' in error) {
    const rawCode = (error as { code: unknown }).code
    if (typeof rawCode === 'string' && /^[A-Z0-9_-]{3,30}$/i.test(rawCode)) {
      code = rawCode
    }
  }

  let rawMsg = 'Unknown error'
  if (error instanceof Error) {
    rawMsg = error.message || error.name
  } else if (typeof error === 'string') {
    rawMsg = error
  } else if (typeof error === 'object' && error !== null && 'message' in error) {
    const m = (error as { message: unknown }).message
    if (typeof m === 'string') rawMsg = m
  }

  // Strip stack trace if present in message (only keep the first error line)
  if (rawMsg.includes('\n')) {
    rawMsg = rawMsg.split('\n')[0]
  }

  let message = sanitizeErrorMessage(rawMsg)
  // Cap length at 200 characters to prevent dumping request/response dumps
  if (message.length > 200) {
    message = message.slice(0, 200) + '... [TRUNCATED]'
  }

  const path = typeof window !== 'undefined' ? window.location.pathname : undefined

  return {
    name,
    message,
    ...(code ? { code } : {}),
    ...(typeof status === 'number' ? { status } : {}),
    ...(path ? { path } : {}),
    timestamp: new Date().toISOString(),
  }
}

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) return undefined
  const response = (error as Record<string, unknown>).response
  if (typeof response !== 'object' || response === null) return undefined
  const status = (response as Record<string, unknown>).status
  return typeof status === 'number' ? status : undefined
}

function isChunkLoadError(error: unknown): boolean {
  if (!error) return false
  const err = error as { name?: string; message?: string }
  const message = typeof err.message === 'string' ? err.message : ''
  const name = typeof err.name === 'string' ? err.name : ''
  return (
    name === 'ChunkLoadError' ||
    message.includes('Loading chunk') ||
    message.includes('Failed to fetch dynamically imported module') ||
    message.includes('error loading dynamically imported module') ||
    message.includes('Importing a module script failed')
  )
}

export function GeneralError({
  className,
  minimal = false,
  error,
}: GeneralErrorProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { history } = useRouter()
  const [copied, setCopied] = useState(false)
  const status = getHttpStatus(error)
  const isChunkError = isChunkLoadError(error)
  const isRateLimited = status === 429

  const title = isChunkError
    ? t('New version available')
    : isRateLimited
      ? t('Too many requests')
      : `${t('Oops! Something went wrong')} :')`

  const description = isChunkError
    ? t('The application has been updated. Please reload the page to load the latest version.')
    : isRateLimited
      ? t('Please wait a moment before trying again.')
      : t('Please try again later.')

  const handleCopy = async () => {
    try {
      const payload = buildSafeErrorPayload(error, status)
      const success = await copyToClipboard(JSON.stringify(payload, null, 2))
      if (success) {
        setCopied(true)
        toast.success(t('Error details copied to clipboard'))
        setTimeout(() => setCopied(false), 2000)
      } else {
        setCopied(false)
        toast.error(t('Failed to copy error details to clipboard'))
      }
    } catch {
      setCopied(false)
      toast.error(t('Failed to copy error details to clipboard'))
    }
  }

  const displayStatus = isChunkError
    ? null
    : status ?? (typeof window !== 'undefined' && window.location.pathname.startsWith('/500') ? 500 : 'Error')

  return (
    <div className={cn('h-svh w-full', className)}>
      <div className='m-auto flex h-full w-full flex-col items-center justify-center gap-2 px-4'>
        {!minimal && (
          <h1 className='text-[5rem] sm:text-[7rem] leading-tight font-bold'>
            {isChunkError ? <RotateCw className='size-20 animate-spin-slow' /> : displayStatus}
          </h1>
        )}
        <span className='font-medium text-lg'>{title}</span>
        <p className='text-muted-foreground text-center max-w-md'>
          {isChunkError ? description : (
            <>
              {t('We apologize for the inconvenience.')} <br /> {description}
            </>
          )}
        </p>
        {!minimal && (
          <div className='mt-6 flex flex-wrap justify-center gap-3'>
            <Button
              variant='default'
              onClick={() => {
                window.location.reload()
              }}
              className='gap-1.5'
            >
              <RotateCw className='size-4' />
              {t('Reload Page')}
            </Button>
            <Button variant='outline' onClick={() => history.go(-1)}>
              {t('Go Back')}
            </Button>
            <Button variant='outline' onClick={() => navigate({ to: '/' })}>
              {t('Back to Home')}
            </Button>
            {!isChunkError && Boolean(error) ? (
              <Button variant='ghost' size='sm' onClick={handleCopy} className='gap-1.5'>
                <Copy className='size-3.5' />
                {copied ? t('Copied') : t('Copy error details')}
              </Button>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}
