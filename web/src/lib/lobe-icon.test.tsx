import { act, cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ loadAdobe: vi.fn() }))

beforeEach(() => {
  vi.resetModules()
  mocks.loadAdobe.mockReset()
  vi.doMock('@lobehub/icons/es/Adobe/index.js', async () => mocks.loadAdobe())
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function adobeModule() {
  const Mono = ({ size }: { size: number }) => (
    <svg data-testid='mono' width={size} />
  )
  Mono.Color = ({ size, filled }: { size: number; filled?: boolean }) => (
    <svg data-testid='color' width={size} data-filled={String(filled)} />
  )
  Mono.Avatar = ({ type }: { type?: string }) => (
    <svg data-testid='avatar' data-type={type} />
  )
  return { default: Mono }
}

describe('on-demand brand icons', () => {
  it('retains the complete catalog, variants and the custom icon', async () => {
    const { getLobeIconNames } = await import('./lobe-icon')
    const names = getLobeIconNames()
    expect(names).toContain('Adobe')
    expect(names).toContain('Adobe.Color')
    expect(names).toContain('OpenAI')
    expect(names).toContain('Sub2API')
    expect(names).toEqual([...new Set(names)].sort())
    expect(mocks.loadAdobe).not.toHaveBeenCalled()
  })

  it('keeps a fixed-size placeholder until visible, then loads only that brand', async () => {
    let notify: IntersectionObserverCallback | undefined
    const disconnect = vi.fn()
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(callback: IntersectionObserverCallback) {
          notify = callback
        }
        observe() {}
        disconnect = disconnect
      }
    )
    mocks.loadAdobe.mockResolvedValue(adobeModule())
    const { getLobeIcon } = await import('./lobe-icon')
    const view = render(getLobeIcon('Adobe.Color.filled=true.size=24', 18))
    expect(view.container.firstElementChild?.getAttribute('style')).toContain(
      'width: 18px'
    )
    expect(mocks.loadAdobe).not.toHaveBeenCalled()
    act(() =>
      notify?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver
      )
    )
    await waitFor(() =>
      expect(view.getByTestId('color').getAttribute('width')).toBe('24')
    )
    expect(view.getByTestId('color').getAttribute('data-filled')).toBe('true')
    expect(disconnect).toHaveBeenCalled()
    expect(mocks.loadAdobe).toHaveBeenCalledTimes(1)
  })

  it('coalesces concurrent loads and reuses a loaded brand for its variants', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    mocks.loadAdobe.mockResolvedValue(adobeModule())
    const { getLobeIcon } = await import('./lobe-icon')
    const view = render(
      <>
        {getLobeIcon('Adobe', 16)}
        {getLobeIcon('Adobe.Color', 20)}
        {getLobeIcon("Adobe.Avatar.type={'platform'}", 20)}
      </>
    )
    await waitFor(() => expect(view.queryByTestId('mono')).not.toBeNull())
    expect(view.getByTestId('color').getAttribute('width')).toBe('20')
    expect(view.getByTestId('avatar').getAttribute('data-type')).toBe(
      'platform'
    )
    expect(mocks.loadAdobe).toHaveBeenCalledTimes(1)
    view.rerender(getLobeIcon('Adobe', 32))
    expect(view.getByTestId('mono').getAttribute('width')).toBe('32')
    expect(mocks.loadAdobe).toHaveBeenCalledTimes(1)
  })

  it('does not let a slow old icon overwrite a changed name or update after unmount', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    let resolve: (value: ReturnType<typeof adobeModule>) => void = () => {}
    mocks.loadAdobe.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done
        })
    )
    const { getLobeIcon } = await import('./lobe-icon')
    const view = render(getLobeIcon('Adobe'))
    await waitFor(() => expect(mocks.loadAdobe).toHaveBeenCalledTimes(1))
    view.rerender(getLobeIcon('NotAnIcon'))
    await act(async () => resolve(adobeModule()))
    expect(view.queryByTestId('mono')).toBeNull()
    expect(view.container.textContent).toBe('N')
    view.unmount()
  })

  it('preserves missing/custom fallbacks without importing arbitrary paths', async () => {
    const { getLobeIcon } = await import('./lobe-icon')
    const view = render(
      <>
        {getLobeIcon(null)}
        {getLobeIcon('  ')}
        {getLobeIcon('../Adobe')}
        {getLobeIcon('Sub2API')}
      </>
    )
    expect(view.container.textContent).toContain('??')
    expect(view.container.querySelector('svg')).not.toBeNull()
    expect(mocks.loadAdobe).not.toHaveBeenCalled()
  })

  it('reports a failed chunk and keeps the rest of the page usable', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    mocks.loadAdobe.mockRejectedValue(new Error('fixture chunk unavailable'))
    const { getLobeIcon } = await import('./lobe-icon')
    const view = render(
      <>
        <p>Page remains usable</p>
        {getLobeIcon('Adobe')}
      </>
    )
    await waitFor(() =>
      expect(
        view.container.querySelector('[data-icon-error="true"]')
      ).not.toBeNull()
    )
    expect(view.getByText('Page remains usable')).toBeTruthy()
    expect(warn).toHaveBeenCalled()
    expect(view.container.textContent).toContain('A')
  })
})
