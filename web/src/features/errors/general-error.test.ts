import { describe, expect, it } from 'vitest'
import {
  buildSafeErrorPayload,
  sanitizeErrorMessage,
} from './general-error'

describe('general-error sanitization and payload tests', () => {
  describe('sanitizeErrorMessage', () => {
    it('redacts password= in plaintext and urls', () => {
      expect(sanitizeErrorMessage('Login error: password=superSecretPassword123')).toBe(
        'Login error: password=[REDACTED]'
      )
      expect(
        sanitizeErrorMessage('Failed url: https://api.example.com/login?password=mysecret&user=admin')
      ).toBe('Failed url: https://api.example.com/login?password=[REDACTED]&user=admin')
    })

    it('redacts Authorization: Basic headers and standalone Basic credentials', () => {
      expect(
        sanitizeErrorMessage('Headers: Authorization: Basic dXNlcjpwYXNzd29yZA==')
      ).toBe('Headers: Authorization: Basic [REDACTED]')

      expect(
        sanitizeErrorMessage('Headers: authorization: basic dXNlcjpwYXNzd29yZA==')
      ).toBe('Headers: authorization: basic [REDACTED]')

      expect(
        sanitizeErrorMessage('Failed credentials Basic dXNlcjpwYXNzd29yZA==')
      ).toBe('Failed credentials Basic [REDACTED]')
    })

    it('redacts ?api_key= and &api_key= in urls and messages', () => {
      expect(
        sanitizeErrorMessage('GET https://llm.internal/v1/models?api_key=sk-1234567890abcdef123456')
      ).toBe('GET https://llm.internal/v1/models?api_key=[REDACTED]')

      expect(
        sanitizeErrorMessage('GET https://llm.internal/v1/chat?model=gpt-4&api_key=secretKey123&token=abc')
      ).toBe('GET https://llm.internal/v1/chat?model=gpt-4&api_key=[REDACTED]&token=[REDACTED]')
    })

    it('redacts sk- API keys, JWTs, and Basic Auth in URL hostnames', () => {
      expect(sanitizeErrorMessage('API Key sk-abcdef1234567890123456 is invalid')).toBe(
        'API Key [REDACTED_API_KEY] is invalid'
      )

      expect(
        sanitizeErrorMessage(
          'Token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.doNotLeakThisSignature'
        )
      ).toBe('Token [REDACTED_JWT]')

      expect(
        sanitizeErrorMessage('Connection string: https://admin:superSecretPass@db.internal:5432/app')
      ).toBe('Connection string: https://admin:[REDACTED]@db.internal:5432/app')
    })

    it('redacts json fields with sensitive keys', () => {
      const jsonStr = '{"username":"admin","password":"mySecretPassword","api_key":"sk-secret"}'
      const sanitized = sanitizeErrorMessage(jsonStr)
      expect(sanitized).not.toContain('mySecretPassword')
      expect(sanitized).not.toContain('sk-secret')
      expect(sanitized).toContain('"password":"[REDACTED]"')
    })
  })

  describe('buildSafeErrorPayload', () => {
    it('returns only allowlist diagnostic fields', () => {
      const customErr = {
        name: 'AxiosError',
        message: 'Request failed with status code 500: password=secretValue123',
        code: 'ERR_BAD_RESPONSE',
        secretInternalConfig: { token: 'supersecret' },
        request: { headers: { Authorization: 'Bearer secret' } },
      }

      const payload = buildSafeErrorPayload(customErr, 500)

      expect(payload).toEqual({
        name: 'AxiosError',
        message: 'Request failed with status code 500: password=[REDACTED]',
        code: 'ERR_BAD_RESPONSE',
        status: 500,
        path: expect.any(String),
        timestamp: expect.any(String),
      })
      expect((payload as unknown as Record<string, unknown>).secretInternalConfig).toBeUndefined()
      expect((payload as unknown as Record<string, unknown>).request).toBeUndefined()
    })

    it('truncates long messages and strips multiline stack traces', () => {
      const multilineErr = new Error(
        'Something broke: password=secret123\n  at Function.execute (/app/server.js:42)\n  at Object.run'
      )
      const payload = buildSafeErrorPayload(multilineErr)

      expect(payload.message).toBe('Something broke: password=[REDACTED]')
      expect(payload.message).not.toContain('/app/server.js')

      const veryLongErr = new Error('A'.repeat(500))
      const truncatedPayload = buildSafeErrorPayload(veryLongErr)
      expect(truncatedPayload.message.length).toBeLessThan(250)
      expect(truncatedPayload.message).toContain('[TRUNCATED]')
    })

    it('handles non-error input safely', () => {
      expect(buildSafeErrorPayload(null)).toMatchObject({
        name: 'Error',
        message: 'Unknown error',
      })
      expect(buildSafeErrorPayload('Plain string error with api_key=12345')).toMatchObject({
        name: 'Error',
        message: 'Plain string error with api_key=[REDACTED]',
      })
    })

    it('verifies safe payload serialization avoids leaking circular or unhandled properties', () => {
      const complexErr = new Error('Database connection failed')
      const payload = buildSafeErrorPayload(complexErr, 503)
      const jsonStr = JSON.stringify(payload)
      const parsed = JSON.parse(jsonStr)

      expect(parsed).toHaveProperty('name', 'Error')
      expect(parsed).toHaveProperty('message', 'Database connection failed')
      expect(parsed).toHaveProperty('status', 503)
      expect(parsed).toHaveProperty('timestamp')
      expect(Object.keys(parsed).every((k) => ['name', 'message', 'code', 'status', 'path', 'timestamp'].includes(k))).toBe(true)
    })
  })
})
