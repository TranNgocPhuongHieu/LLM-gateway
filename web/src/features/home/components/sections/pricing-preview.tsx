import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import { getDynamicDisplayGroupRatio, getDynamicPricingSummary } from '@/features/pricing/lib/dynamic-price'
import { formatPrice } from '@/features/pricing/lib/price'
import { resolveAppLocale } from '@/lib/currency'
import { getPromotionAnnouncement } from '@/lib/api'
import { useQuery } from '@tanstack/react-query'

const HOME_PRICING_MODEL_ORDER = [
  'gpt-6.1-sol',
  'gpt-6-luna',
  'claude-opus-5-5',
  'gpt-6-astra',
] as const

// Reuse the public tariff formatters; screenshot prices are not a price source.
export function PricingPreview() {
  const { t } = useTranslation()
  const { models, isLoading, error, priceRate, usdExchangeRate } = usePricingData()
  const promotionQuery = useQuery({ queryKey: ['promotion-announcement'], queryFn: getPromotionAnnouncement, staleTime: 60_000 })
  const promotions = Array.isArray(promotionQuery.data?.data) ? promotionQuery.data.data.filter((p): p is NonNullable<typeof p> => Boolean(p) && typeof p === 'object' && typeof p.type === 'string') : []
  const featuredModels = HOME_PRICING_MODEL_ORDER.flatMap((modelName) => {
    const model = models.find((candidate) => candidate.model_name === modelName)
    return model ? [model] : []
  })
  return (
    <section id='pricing-preview' className='px-5 py-12 sm:px-6'>
      <div className='mx-auto max-w-6xl'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <h2 className='text-2xl font-semibold'>{t('Transparent pricing')}</h2>
          <Link to='/pricing' className='text-primary text-sm'>{t('View models')} →</Link>
        </div>
        <p className='text-muted-foreground mt-2 text-sm'>{t('Pay according to usage, monitor in real time')} · {t('Credit per 1M tokens')}</p>
        <p className='text-muted-foreground/80 mt-1 text-xs'>{t('Credit conversion reference: 1 credit = 1,000 VND ≈ ${{usd}}', { usd: new Intl.NumberFormat(resolveAppLocale(), { minimumFractionDigits: 4, maximumFractionDigits: 4 }).format(1000 / Math.max(usdExchangeRate, 1)) })}</p>
        {promotions.filter(p => p.type === 'model').map(p => <div key={p.summary} className='mt-4 flex items-center gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4'><span className='rounded-full bg-[#3d2c0c] px-3 py-1 text-xs font-bold text-[#ffbf47]'>{t('Discount {{percent}}%', { percent: p.discount_percent })}</span><div><strong>{t('Discount {{percent}}% for {{count}} models', { percent: p.discount_percent, count: p.models?.includes('*') ? t('all models') : p.models?.length ?? 0 })}</strong><p className='text-muted-foreground text-xs'>{t('The promotion applies for a limited time.')}</p></div></div>)}
        <div className='bg-card mt-6 overflow-x-auto rounded-2xl border p-4'>
          {isLoading ? <p className='p-3'>{t('Loading...')}</p> : error ? (
            <p role='alert' className='p-3 text-destructive'>{t('Failed to load pricing data')}</p>
          ) : (
            <table className='w-full min-w-[640px] text-left text-sm'>
              <thead className='text-muted-foreground'><tr>
                <th className='p-3 font-medium'>{t('Model')}</th>
                <th className='p-3 text-right font-medium'>{t('Input')} <span className='text-xs font-normal'>({t('credit')} / 1M token)</span></th>
                <th className='p-3 text-right font-medium'>{t('Cache-read')} <span className='text-xs font-normal'>({t('credit')} / 1M token)</span></th>
                <th className='p-3 text-right font-medium'>{t('Output')} <span className='text-xs font-normal'>({t('credit')} / 1M token)</span></th>
              </tr></thead>
              <tbody>
                {featuredModels.map((model) => {
                  const groupRatioMultiplier = getDynamicDisplayGroupRatio(model)
                  const summary = getDynamicPricingSummary(model, { tokenUnit: 'M', priceRate, usdExchangeRate, groupRatioMultiplier })
                  const price = (type: 'input' | 'output' | 'cache', field: string) => {
                    if (!summary) return formatPrice(model, type, 'M', false, priceRate, usdExchangeRate)
                    const entry = summary.entries.find((item) => item.field === field)
                    return (entry?.formattedRange || entry?.formatted || '—').replace(/\scredit\b/g, '')
                  }
                  const sale = promotions.find(p => p.type === 'model' && (p.models?.includes('*') || p.models?.includes(model.model_name)))
                  const shown = (value: string) => sale ? <><span className='text-muted-foreground line-through'>{value}</span><span className='mx-1'>→</span><span className='font-semibold text-blue-300'>{discountPrice(value, sale.discount_percent ?? 0)}</span></> : value
                  return <tr key={model.model_name} className='border-t'>
                    <td className='p-3 font-mono text-xs'>{model.model_name}{sale && <span className='ms-2 rounded-full bg-[#3d2c0c] px-2 py-1 font-sans text-[10px] font-bold text-[#ffbf47]'>{t('Discount {{percent}}%', { percent: sale.discount_percent })}</span>}</td>
                    <td className='p-3 text-right tabular-nums'>{shown(price('input', 'inputPrice'))}</td>
                    <td className='p-3 text-right tabular-nums'>{shown(price('cache', 'cacheReadPrice'))}</td>
                    <td className='p-3 text-right tabular-nums'>{shown(price('output', 'outputPrice'))}</td>
                  </tr>
                })}
              </tbody>
            </table>
          )}
          {!isLoading && !error && featuredModels.length === 0 && <p className='p-3 text-muted-foreground'>{t('No data')}</p>}
        </div>
      </div>
    </section>
  )
}

function discountPrice(value: string, percent: number) {
  const match = value.match(/-?\d+(?:[,.]\d+)?/)
  if (!match) return value
  const n = Number(match[0].replace(',', '.')) * (1 - percent / 100)
  return n.toLocaleString('vi-VN', { maximumFractionDigits: 4 })
}
