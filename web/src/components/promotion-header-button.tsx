import { Tag } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { getPromotionAnnouncement } from '@/lib/api'

type Promotion = {
  type: string
  status: 'active' | 'upcoming'
  title: string
  summary: string
  min_amount?: number
  bonus_percent?: number
  discount_percent?: number
  models?: string[]
  start_at: string
  end_at: string
}

function formatDate(value: string, locale: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale, {
    day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
    timeZone: 'Asia/Ho_Chi_Minh',
  }).format(date)
}

function promotionSummary(promotion: Promotion, t: ReturnType<typeof useTranslation>['t'], locale: string) {
  if (promotion.type === 'model') return t('Discount {{percent}}% for {{count}} models', {
    percent: promotion.discount_percent,
    count: promotion.models?.includes('*') ? t('all models') : promotion.models?.length ?? 0,
  })
  return t('Top up from {{amount}} VND for {{percent}}% extra credit', {
    amount: new Intl.NumberFormat(locale).format(promotion.min_amount ?? 0), percent: promotion.bonus_percent,
  })
}

function PromotionCard({ promotion, onNavigate }: { promotion: Promotion; onNavigate: () => void }) {
  const { t, i18n } = useTranslation()
  return (
    <div className='rounded-xl border bg-card p-5'>
      <div className='flex items-start justify-between gap-4'>
        <div className='min-w-0'>
          <h3 className='font-semibold'>{t(promotion.type === 'model' ? 'Model promotions' : 'Top-up promotions')}</h3>
          <p className='mt-2 text-lg font-semibold'>{promotionSummary(promotion, t, i18n.language)}</p>
          {promotion.type === 'model' && promotion.models?.length ? <div className='mt-2 flex flex-wrap gap-2'>{promotion.models.map(model => <span className='break-all rounded bg-muted px-2 py-1 font-mono text-xs' key={model}>{model === '*' ? t('all models') : model}</span>)}</div> : null}
          <p className='mt-2 text-sm text-muted-foreground'>
            {promotion.start_at ? `${t('Promotion starts')}: ${formatDate(promotion.start_at, i18n.language)}` : ''}
            {promotion.end_at ? ` · ${t('Promotion ends')}: ${formatDate(promotion.end_at, i18n.language)}` : ''}
            {(promotion.start_at || promotion.end_at) && ` (${t('Vietnam time')})`}
          </p>
        </div>
        <span className={promotion.status === 'active' ? 'shrink-0 rounded-full bg-success/15 px-3 py-1 text-xs font-semibold text-success' : 'shrink-0 rounded-full bg-warning/15 px-3 py-1 text-xs font-semibold text-warning'}>
          {t(promotion.status === 'active' ? 'Promotion active' : 'Promotion upcoming')}
        </span>
      </div>
      <Link to={promotion.type === 'model' ? '/pricing' : '/wallet'} onClick={onNavigate} className='mt-4 inline-flex text-sm font-semibold text-primary hover:underline'>
        {t(promotion.type === 'model' ? 'View promotion pricing' : 'Top up for promotion')} →
      </Link>
    </div>
  )
}

export function PromotionHeaderButton() {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  const query = useQuery({ queryKey: ['promotion-announcement'], queryFn: getPromotionAnnouncement, staleTime: 60_000, refetchInterval: 60_000 })
  const promotions = query.data?.success && Array.isArray(query.data.data) ? query.data.data.filter((item): item is Promotion => Boolean(item) && ['model', 'topup'].includes(item.type) && ['active', 'upcoming'].includes(item.status) && (!item.end_at || Date.parse(item.end_at) > now)).map(item => ({...item, status: (item.start_at && Date.parse(item.start_at) > now ? 'upcoming' : 'active') as Promotion['status']})) : []
  if (promotions.length === 0) return null
  const activeCount = promotions.filter((item) => item.status === 'active').length
  const label = promotions.length === 1 ? `${promotions[0].status === 'upcoming' ? `${t('Promotion upcoming')} · ` : ''}${promotionSummary(promotions[0], t, i18n.language)}` : t('{{count}} available promotions', { count: promotions.length })
  return (
    <Dialog open={open} onOpenChange={setOpen} title={t('Promotions')} description={t('Promotions apply automatically; no code needed.')} contentClassName='sm:max-w-3xl' bodyClassName='space-y-4' trigger={<Button variant='outline' className='relative h-9 max-w-[16rem] gap-2 border-warning/30 bg-warning/10 text-warning hover:bg-warning/15' aria-label={label} title={label}><span aria-hidden className={`size-2 shrink-0 rounded-full ${activeCount ? 'bg-success' : 'bg-warning'}`} /><Tag className='size-4 sm:hidden' /><span className='hidden truncate sm:inline'>{label} ›</span></Button>}>
      {promotions.map((promotion) => <PromotionCard key={`${promotion.type}:${promotion.summary}:${promotion.start_at}`} promotion={promotion} onNavigate={() => setOpen(false)} />)}
    </Dialog>
  )
}
