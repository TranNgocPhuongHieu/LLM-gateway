import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { quotaUnitsToDollars } from '@/lib/format'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import type { UserWalletData } from '../types'

export function WalletStatsCard({ user, loading }: { user: UserWalletData | null; loading?: boolean }) {
  const { i18n } = useTranslation()
  const vi = normalizeInterfaceLanguage(i18n.resolvedLanguage || i18n.language) === 'vi'
  const credits = quotaUnitsToDollars(user?.quota ?? 0)
  const format = (n: number, digits: number) => new Intl.NumberFormat(vi ? 'vi-VN' : 'en-US', { maximumFractionDigits: digits }).format(n)
  return <section className='rounded-2xl border bg-card p-5 sm:p-7'>
    <h2 className='text-sm font-medium text-muted-foreground'>{vi ? 'Số dư hiện tại' : 'Current balance'}</h2>
    {loading ? <Skeleton className='mt-3 h-10 w-56' /> : <><p className='mt-2 text-3xl font-bold tracking-tight tabular-nums'>{format(credits, 2)} credit</p><p className='mt-1 text-sm text-muted-foreground'>≈ {format(credits * 1000, 0)} VND</p></>}
  </section>
}
