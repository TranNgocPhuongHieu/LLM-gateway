import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { normalizeInterfaceLanguage } from '@/i18n/languages'

import { getDocPages } from './content'

const DOC_GROUPS = [
  { key: 'start', slugs: ['', 'quickstart', 'api-keys', 'balance'] },
  { key: 'integrations', slugs: ['codex', 'claude-code', 'kilo-code', 'roo-code'] },
  { key: 'sdk', slugs: ['openai-sdk', 'anthropic-sdk', 'api-reference', 'models-pricing'] },
  { key: 'operations', slugs: ['streaming', 'troubleshooting', 'changelog'] },
] as const

export function Docs({ slug = '' }: { slug?: string }) {
  const { i18n } = useTranslation()
  const vi = normalizeInterfaceLanguage(i18n.resolvedLanguage || i18n.language) === 'vi'
  const s = (vn: string, en: string) => vi ? vn : en
  const pages = getDocPages(vi)
  const [query, setQuery] = useState('')
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [copyError, setCopyError] = useState(false)
  const current = pages.findIndex((page) => page.slug === slug)
  const page = pages[current]
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const results = pages.filter((item) => [item.title, item.summary, ...item.blocks.map((block) => `${block.heading || ''} ${block.text || ''} ${block.code || ''}`)].join(' ').toLocaleLowerCase().includes(normalizedQuery))
  const groupedResults = DOC_GROUPS.map((group) => ({
    ...group,
    pages: group.slugs.map((groupSlug) => results.find((item) => item.slug === groupSlug)).filter(Boolean),
  })).filter((group) => group.pages.length > 0)
  const groupTitle = (key: typeof DOC_GROUPS[number]['key']) => ({
    start: s('Bắt đầu', 'Getting started'),
    integrations: s('Tích hợp ứng dụng', 'Application integrations'),
    sdk: s('SDK & API', 'SDK & API'),
    operations: s('Vận hành & hỗ trợ', 'Operations & support'),
  })[key]
  const docLink = (item: typeof pages[number], children: React.ReactNode, className?: string) => item.slug
    ? <Link to='/docs/$page' params={{ page: item.slug }} className={className} onClick={() => setQuery('')}>{children}</Link>
    : <Link to='/docs' className={className} onClick={() => setQuery('')}>{children}</Link>

  return <PublicLayout showMainContainer={false}>
    <main className='mx-auto max-w-7xl px-5 pb-12 pt-24 sm:px-6'>
      <div className='mb-8 rounded-2xl border bg-gradient-to-br from-primary/10 to-background p-6'>
        <p className='text-sm font-medium text-primary'>{s('Trung tâm hướng dẫn', 'Developer guides')}</p>
        <h1 className='mt-2 text-3xl font-semibold tracking-tight'>{page?.title || s('Không tìm thấy tài liệu', 'Documentation not found')}</h1>
        <p className='mt-3 text-muted-foreground'>{page?.summary || s('Chọn một mục trong menu để tiếp tục.', 'Choose a guide from the menu to continue.')}</p>
      </div>
      <div className='grid items-start gap-8 lg:grid-cols-[240px_minmax(0,1fr)]'>
        <aside className='rounded-xl border bg-card p-4 lg:sticky lg:top-24'>
          <label htmlFor='docs-search' className='mb-2 block text-sm font-medium'>{s('Tìm tài liệu', 'Search documentation')}</label>
          <Input id='docs-search' type='search' value={query} onChange={(event) => setQuery(event.target.value)} placeholder={s('Model, key, lỗi…', 'Model, key, errors…')} />
          <nav aria-label={s('Mục lục tài liệu', 'Documentation navigation')} className='mt-3 flex max-h-64 flex-col gap-1 overflow-y-auto lg:max-h-[65vh]'>
            {groupedResults.map((group) => <section key={group.key} className='mb-4 last:mb-0'>
              <h2 className='px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-foreground'>{groupTitle(group.key)}</h2>
              <div className='flex flex-col gap-1'>{group.pages.map((item) => item && <div key={item.slug}>{docLink(item, item.title, `block rounded-lg px-3 py-2 text-sm ${item.slug === slug ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted hover:text-foreground'}`)}</div>)}</div>
            </section>)}
            {results.length === 0 && <p role='status' className='p-3 text-sm text-muted-foreground'>{s('Không có kết quả.', 'No results.')}</p>}
          </nav>
        </aside>
        <article className='min-w-0 rounded-2xl border bg-card p-5 sm:p-8' aria-label={page?.title}>
          {copyError && <p role='alert' className='mb-4 text-destructive'>{s('Không sao chép được. Hãy chọn và sao chép đoạn mã thủ công.', 'Copy failed. Select and copy the code manually.')}</p>}
          {page?.blocks.map((block, index) => <section key={index} className='mb-7 last:mb-0'>
            {block.heading && <h2 className='mb-3 text-lg font-semibold'>{block.heading}</h2>}
            {block.text && <p className='leading-7 text-muted-foreground'>{block.text}</p>}
            {block.code && <div className='overflow-hidden rounded-xl border bg-muted/40'>
              <div className='flex items-center justify-between gap-3 border-b px-4 py-2'>
                <span className='text-xs text-muted-foreground'>{block.language || s('Cấu hình', 'Configuration')}</span>
                <Button size='sm' variant='ghost' aria-label={s('Sao chép đoạn mã', 'Copy code')} onClick={async () => { setCopyError(!await copyToClipboard(block.code!)) }}>{copiedText === block.code ? s('Đã sao chép', 'Copied') : s('Sao chép', 'Copy')}</Button>
              </div>
              <pre className='overflow-x-auto p-4 text-sm leading-6' tabIndex={0}><code>{block.code}</code></pre>
            </div>}
            {block.href && <a className='font-medium text-primary underline underline-offset-4' href={block.href} {...(block.href.startsWith('https://') ? { target: '_blank', rel: 'noopener noreferrer' } : {})}>{block.label}</a>}
          </section>)}
          {!page && <Link to='/docs' className='text-primary'>{s('Về tài liệu', 'Back to documentation')}</Link>}
          {page && <nav aria-label={s('Chuyển bài', 'Page navigation')} className='mt-8 flex flex-wrap justify-between gap-4 border-t pt-5 text-sm text-primary'>
            {current > 0 ? docLink(pages[current - 1], `← ${pages[current - 1].title}`) : <span />}
            {current < pages.length - 1 && docLink(pages[current + 1], `${pages[current + 1].title} →`)}
          </nav>}
        </article>
      </div>
    </main>
    <Footer />
  </PublicLayout>
}
