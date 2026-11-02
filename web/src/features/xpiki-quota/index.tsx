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

import { useMutation, useQuery } from '@tanstack/react-query'
import {
  Calendar as CalendarIcon,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  Clock,
  Coins,
  ExternalLink,
  KeyRound,
  Plus,
  Radio,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable, type StaticDataTableColumn } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout/components/section-page-layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { MultiSelect } from '@/components/multi-select'

import {
  checkXPikiQuota,
  checkKiroQuota,
  fetchXPikiLogs,
  updateChannelStatus,
  type CustomKeyItem,
  type XPikiLogItem,
  type XPikiQuotaKeyResult,
  type XPikiQuotaModelProfile,
  type XPikiQuotaReport,
  type XPikiKeyOption,
  type KiroQuotaReport,
} from './api'
import { formatPoolTokens, referenceTokenPools } from './token-pools'
import { actualTariffRows, formatLogCost, formatXPikiVND, tokenCostExplanation } from './token-cost'
import { STORAGE_KEY_CACHE, STORAGE_KEY_CUSTOM_KEYS, STORAGE_KEY_LOGS, CachedQuotaData, quotaRows } from './xpiki-quota-utils'

const STORAGE_KEY_KIRO = 'newapi_kiro_quota_keys_v1'
const viNumber = new Intl.NumberFormat('vi-VN', { maximumFractionDigits: 1 })
const viInteger = new Intl.NumberFormat('vi-VN', { maximumFractionDigits: 0 })
const formatPercent = (value: number) => `${viNumber.format(Math.max(0, Math.min(100, value)))}%`

function KiroQuotaCard(props: { report: KiroQuotaReport | null; onReport: (report: KiroQuotaReport) => void; checkSignal: number; tokenKeys: CustomKeyItem[] }) {
  const { report, onReport } = props
  const [keys, setKeys] = useState<CustomKeyItem[]>(() => {
    try { return JSON.parse(localStorage.getItem(STORAGE_KEY_KIRO) || '[]') } catch { return [] }
  })
  const [draft, setDraft] = useState('')
  const [showInput, setShowInput] = useState(false)
  const mutation = useMutation({
    mutationFn: () => checkKiroQuota([...keys, ...props.tokenKeys]),
    retry: false,
    onSuccess: onReport,
    onError: (error: unknown) => handleServerError(error, 'Kiro quota check failed'),
  })
  useEffect(() => { if (props.checkSignal > 0) mutation.mutate() }, [props.checkSignal])
  const save = (next: CustomKeyItem[]) => {
    setKeys(next)
    localStorage.setItem(STORAGE_KEY_KIRO, JSON.stringify(next))
  }
  const add = () => {
    const key = draft.trim()
    if (!key) return
    save([...keys, { key, label: `Kiro Key ${keys.length + 1}` }])
    setDraft('')
  }
  const totalLimit = report ? (report.total_limit || report.total_used + report.total_remaining) : 0
  const totalPercent = totalLimit && report ? (report.total_used / totalLimit) * 100 : 0
  const remainingPercent = 100 - totalPercent
  const status = remainingPercent < 10 ? 'Nguy cấp' : remainingPercent < 30 ? 'Sắp hết' : 'Còn ổn'
  const statusClass = remainingPercent < 10 ? 'bg-[#3a1519] text-[#ff8f8f]' : remainingPercent < 30 ? 'bg-[#3d2c0c] text-[#ffbf47]' : 'bg-[#11382a] text-[#5be5a8]'
  return <Card className='flex h-full flex-col rounded-2xl border-[#26282d] bg-[#17181b] p-2'>
    <CardHeader className='pb-2'>
      <div className='flex items-center justify-between gap-2'><CardTitle>Kiro</CardTitle><div className='flex items-center gap-2'><Badge className={cn('rounded-full border-0 px-3', statusClass)}>{status}</Badge><Button size='icon' variant='ghost' aria-label='Kiểm tra Kiro' onClick={() => mutation.mutate()} disabled={mutation.isPending}><RefreshCw className={cn('size-4', mutation.isPending && 'animate-spin')} /></Button></div></div>
      <CardDescription>Còn lại · kiro.pix4k.com</CardDescription>
      <div className='flex items-baseline gap-2 text-4xl font-bold tabular-nums tracking-tight'>{report ? viInteger.format(report.total_remaining) : '—'} <span className='text-base font-medium text-muted-foreground'>credit</span></div>
    </CardHeader>
    <CardContent className='flex flex-1 flex-col space-y-3'>
      <div className='h-2 overflow-hidden rounded-full bg-[#2a2d33]'><div className={cn('h-full rounded-full', remainingPercent < 10 ? 'bg-[#ff6b6b]' : remainingPercent < 30 ? 'bg-[#f5a524]' : 'bg-[#3ddc97]')} style={{ width: `${Math.min(100, Math.max(0, remainingPercent))}%` }} /></div>
      <div className='flex justify-between text-xs text-muted-foreground'><span>{report ? `${formatPercent(remainingPercent)} còn lại` : 'Chưa kiểm tra'}</span><span>Tổng {viInteger.format(totalLimit || 0)}</span></div>
      <div className='grid grid-cols-3 gap-3 border-t pt-3 text-sm'><div><div className='text-xs text-muted-foreground'>Đã dùng</div><div className='font-semibold tabular-nums'>{report ? viInteger.format(report.total_used) : '—'}</div></div><div><div className='text-xs text-muted-foreground'>Mức dùng</div><div className='font-semibold tabular-nums'>{report ? formatPercent(totalPercent) : '—'}</div></div><div><div className='text-xs text-muted-foreground'>Khóa hoạt động</div><div className='font-semibold'>{report ? `${report.keys.filter((key) => !key.error).length} / ${report.keys.length}` : '—'}</div></div></div>
      <div className='mt-auto flex items-center justify-between gap-2 pt-4'><span className='text-xs text-muted-foreground'>Key chỉ lưu trên trình duyệt này.</span><div>{showInput ? <div className='flex gap-2'><Input autoFocus className='h-8 w-28' type='password' value={draft} onChange={(e) => setDraft(e.target.value)} placeholder='Key' onKeyDown={(e) => e.key === 'Enter' && add()} /><Button size='sm' variant='outline' onClick={add}>Thêm</Button></div> : <Button size='sm' variant='link' className='px-0' onClick={() => setShowInput(true)}>+ Thêm key</Button>}</div></div>
    </CardContent>
  </Card>
}

function KiroTokenQuotaCard(props: { report: KiroQuotaReport | null; onCheckKey: (key: string) => void }) {
  const { report } = props
  const [draft, setDraft] = useState('')
  const tokenKeys = report?.keys.filter((key) => key.quota_mode === 'token' && !key.error) ?? []
  const remaining = tokenKeys.reduce((sum, key) => sum + (key.remaining_tokens ?? 0), 0)
  const limit = tokenKeys.reduce((sum, key) => sum + (key.limit_tokens ?? 0), 0)
  const used = tokenKeys.reduce((sum, key) => sum + (key.used_tokens ?? 0), 0)
  const percent = limit > 0 ? (used / limit) * 100 : 0
  const hasErrors = report?.keys.some((key) => key.error) ?? false
  return <Card className='flex h-full flex-col rounded-2xl border-[#26282d] bg-[#17181b] p-2'>
    <CardHeader className='pb-2'><div className='flex items-center justify-between gap-2'><CardTitle>Kiro Token</CardTitle><Badge className={cn('rounded-full border-0 px-3', percent >= 90 ? 'bg-[#3a1519] text-[#ff8f8f]' : percent >= 70 ? 'bg-[#3d2c0c] text-[#ffbf47]' : 'bg-[#11382a] text-[#5be5a8]')}>{tokenKeys.length ? (percent >= 90 ? 'Nguy cấp' : percent >= 70 ? 'Sắp hết' : 'Còn ổn') : 'Chưa có key'}</Badge></div><CardDescription>Còn lại · tính theo token</CardDescription><div className='flex items-baseline gap-2 text-4xl font-bold tabular-nums tracking-tight'>{tokenKeys.length ? viInteger.format(remaining) : '—'} <span className='text-base font-medium text-muted-foreground'>token</span></div></CardHeader>
    <CardContent className='flex flex-1 flex-col space-y-3'><div className='h-2 overflow-hidden rounded-full bg-[#2a2d33]'><div className='h-full rounded-full bg-[#3ddc97]' style={{ width: `${Math.min(100, Math.max(0, 100 - percent))}%` }} /></div><div className='flex justify-between text-xs text-muted-foreground'><span>{tokenKeys.length ? `${formatPercent(100 - percent)} còn lại` : 'Chưa kiểm tra'}</span><span>Tổng {viInteger.format(limit)}</span></div><div className='grid grid-cols-3 gap-3 border-t pt-3 text-sm'><div><div className='text-xs text-muted-foreground'>Đã dùng</div><div className='font-semibold tabular-nums'>{tokenKeys.length ? viInteger.format(used) : '—'}</div></div><div><div className='text-xs text-muted-foreground'>Mức dùng</div><div className='font-semibold tabular-nums'>{tokenKeys.length ? formatPercent(percent) : '—'}</div></div><div><div className='text-xs text-muted-foreground'>Khóa hoạt động</div><div className='font-semibold'>{tokenKeys.length} / {report?.keys.filter((key) => key.quota_mode === 'token').length ?? 0}</div></div></div><div className='mt-auto pt-4 text-xs text-muted-foreground'>Key token dùng chung thao tác kiểm tra Kiro; không cộng vào card credit.</div></CardContent>
    <CardContent className='space-y-2'>
      {hasErrors && <p role='alert' className='text-sm text-destructive'>Có khóa kiểm tra thất bại; số liệu có thể chưa đầy đủ. Xem chi tiết theo khóa.</p>}
      <form className='flex gap-2' onSubmit={(event) => { event.preventDefault(); props.onCheckKey(draft.trim()); setDraft('') }}>
        <Input type='password' aria-label='Key Kiro Token' placeholder='Key Kiro Token' value={draft} onChange={(event) => setDraft(event.target.value)} autoComplete='off' />
        <Button type='submit' variant='outline'>Kiểm tra token</Button>
      </form>
      <p className='text-xs text-muted-foreground'>Key nhập tại đây chỉ giữ trong bộ nhớ đến khi tải lại trang.</p>
    </CardContent>
  </Card>
}

function XPikiSourceCard(props: {
  report: XPikiQuotaReport | null
  onCheck: () => void
  pending: boolean
}) {
  const { report, onCheck, pending } = props
  const pools = report ? referenceTokenPools(report) : []
  const pool = pools.find((item) => item.model === 'claude-opus-5-5') ?? pools[0]
  const remaining = pool && !pool.unlimited ? formatPoolTokens(pool.remaining) : '—'
  const used = pool && !pool.unlimited ? formatPoolTokens(pool.used) : '—'
  const limit = pool && !pool.unlimited ? formatPoolTokens(pool.limit) : '—'
  const remainingPercent = pool && pool.limit ? Number(pool.remaining) / Number(pool.limit) * 100 : 0
  const status = remainingPercent < 10 ? 'Nguy cấp' : remainingPercent < 30 ? 'Sắp hết' : 'Còn ổn'
  const statusClass = remainingPercent < 10 ? 'bg-[#3a1519] text-[#ff8f8f]' : remainingPercent < 30 ? 'bg-[#3d2c0c] text-[#ffbf47]' : 'bg-[#11382a] text-[#5be5a8]'
  return (
    <Card className='flex h-full flex-col rounded-2xl border-[#26282d] bg-[#17181b] p-2'>
      <CardHeader>
        <div className='flex items-center justify-between gap-2'><CardTitle>XPiki</CardTitle><div className='flex items-center gap-2'><Badge className={cn('rounded-full border-0 px-3', statusClass)}>{status}</Badge><Button size='icon' variant='ghost' aria-label='Kiểm tra XPiki' onClick={onCheck} disabled={pending}><RefreshCw className={cn('size-4', pending && 'animate-spin')} /></Button></div></div>
        <CardDescription>Còn lại · {pool?.label ?? 'Claude Opus 5.5'}</CardDescription>
        <div className='flex items-baseline gap-2 text-4xl font-bold tabular-nums tracking-tight'>{remaining} <span className='text-base font-medium text-muted-foreground'>token</span></div>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col space-y-4'>
        <div className='h-2 overflow-hidden rounded-full bg-[#2a2d33]'><div className={cn('h-full rounded-full', remainingPercent < 10 ? 'bg-[#ff6b6b]' : remainingPercent < 30 ? 'bg-[#f5a524]' : 'bg-[#3ddc97]')} style={{ width: `${Math.min(100, Math.max(0, remainingPercent))}%` }} /></div>
        <div className='flex justify-between text-xs text-muted-foreground'><span>{remainingPercent ? `${formatPercent(remainingPercent)} còn lại` : 'Chưa kiểm tra'}</span><span>Giới hạn {limit}</span></div>
        <div className='grid grid-cols-3 gap-3 border-t pt-3 text-sm'><div><div className='text-xs text-muted-foreground'>Đã dùng</div><div className='font-semibold tabular-nums'>{used}</div></div><div><div className='text-xs text-muted-foreground'>Mức dùng</div><div className='font-semibold tabular-nums'>{remainingPercent ? formatPercent(100 - remainingPercent) : '—'}</div></div><div><div className='text-xs text-muted-foreground'>Khóa hoạt động</div><div className='font-semibold'>{report ? `${report.keys.filter((key) => !key.error).length} / ${report.keys.length}` : '—'}</div></div></div>
        <div className='mt-auto pt-4 text-xs text-muted-foreground'>GPT tính theo token Astra; Claude tính theo token Opus 5.5. Không cộng lẫn hai loại token.</div>
      </CardContent>
    </Card>
  )
}

function SourceKeyDetails(props: { xpiki: XPikiQuotaReport | null; kiro: KiroQuotaReport | null }) {
  const rows = [
    ...(props.xpiki?.keys ?? []).map((key) => ({
      label: key.label,
      detail: key.name,
      channelId: key.channel_id,
      source: 'XPiki',
      remaining: key.remaining_tokens ? `${new Intl.NumberFormat('vi-VN').format(Number(key.remaining_tokens.replace(/,/g, '')))} token` : key.remaining_vnd || '—',
      limit: key.limit_tokens ? ` / ${new Intl.NumberFormat('vi-VN').format(Number(key.limit_tokens.replace(/,/g, '')))} token` : '',
      percent: key.used_percent ? `${Math.max(0, 100 - Number.parseFloat(key.used_percent))}%` : '—',
      status: key.error ? key.error : (Number.parseFloat(key.used_percent) >= 90 ? 'Nguy cấp' : Number.parseFloat(key.used_percent) >= 70 ? 'Sắp hết' : 'Còn ổn'),
    })),
    ...(props.kiro?.keys ?? []).filter((key) => key.quota_mode !== 'token').map((key) => ({
      label: key.label,
      detail: key.name,
      channelId: undefined,
      source: 'Kiro',
      remaining: key.unlimited ? '∞' : `${viInteger.format(key.remaining_credits)} credit`,
      limit: key.unlimited ? '' : ` / ${viInteger.format(key.limit_credits)} credit`,
      percent: key.unlimited ? '∞' : formatPercent(100 - key.used_percent),
      status: key.error ? key.error : (key.used_percent >= 90 ? 'Nguy cấp' : key.used_percent >= 70 ? 'Sắp hết' : 'Còn ổn'),
    })),
    ...(props.kiro?.keys ?? []).filter((key) => key.quota_mode === 'token').map((key) => ({
      label: key.label, detail: key.name, channelId: undefined, source: 'Kiro Token',
      remaining: `${viInteger.format(key.remaining_tokens ?? 0)} token`, limit: ` / ${viInteger.format(key.limit_tokens ?? 0)} token`,
      percent: formatPercent(100 - (key.used_percent || 0)), status: key.error ? key.error : ((key.used_percent || 0) >= 90 ? 'Nguy cấp' : (key.used_percent || 0) >= 70 ? 'Sắp hết' : 'Còn ổn'),
    })),
  ]
  if (!rows.length) return null
  return <Card className='rounded-2xl border-[#26282d] bg-[#17181b] p-2 shadow-none'>
    <CardHeader><CardTitle>Chi tiết theo khóa</CardTitle><CardDescription>{rows.length} khóa · {new Set(rows.map((row) => row.source)).size} nguồn · sắp xếp theo mức còn lại thấp nhất</CardDescription></CardHeader>
    <CardContent className='overflow-x-auto'>
      <table className='w-full min-w-[720px] text-sm'><thead><tr className='border-b text-left text-muted-foreground'><th className='px-3 py-2 font-medium'>Khóa</th><th className='px-3 py-2 font-medium'>Nguồn</th><th className='px-3 py-2 font-medium'>Còn lại</th><th className='px-3 py-2 font-medium'>Mức còn lại</th><th className='px-3 py-2 text-right font-medium'>Trạng thái</th></tr></thead><tbody>{rows.sort((a, b) => Number.parseFloat(a.percent) - Number.parseFloat(b.percent)).map((row) => { const pct = Number.parseFloat(row.percent); const tone = pct < 10 ? ['bg-[#3a1519]', 'text-[#ff8f8f]', 'bg-[#ff6b6b]'] : pct < 30 ? ['bg-[#3d2c0c]', 'text-[#ffbf47]', 'bg-[#f5a524]'] : ['bg-[#11382a]', 'text-[#5be5a8]', 'bg-[#3ddc97]']; return <tr key={`${row.source}-${row.label}`} className='border-b last:border-0'><td className='px-3 py-3 font-medium'>{row.channelId ? <a href='/channels' className='text-primary hover:underline'>{row.label}</a> : row.label}{row.detail && <div className='max-w-[180px] truncate text-xs font-normal text-muted-foreground' title={row.detail}>{row.detail}</div>}</td><td className='px-3 py-3'><Badge variant='secondary'>{row.source}</Badge></td><td className='px-3 py-3 font-semibold tabular-nums'><div>{row.remaining}</div><div className='text-xs font-normal text-muted-foreground'>{row.limit}</div></td><td className='px-3 py-3'><div className='flex items-center gap-2'><div className='h-2 w-28 overflow-hidden rounded-full bg-[#2a2d33]'><div className={cn('h-full rounded-full', tone[2])} style={{ width: `${Math.min(100, Math.max(0, pct || 0))}%` }} /></div><span className='tabular-nums'>{row.percent}</span></div></td><td className='px-3 py-3 text-right'><Badge className={cn('rounded-full border-0', tone[0], tone[1])}>{row.status}</Badge></td></tr>})}</tbody></table>
    </CardContent>
  </Card>
}

function StatusBadge(props: {
  keyResult: XPikiQuotaKeyResult
  translate: (value: string) => string
}) {
  const { keyResult, translate } = props
  if (keyResult.error) {
    return (
      <Badge
        variant='destructive'
        className='max-w-[220px] truncate'
        title={keyResult.error}
      >
        {keyResult.error}
      </Badge>
    )
  }

  const statuses = keyResult.status || []
  if (statuses.includes('quota_exhausted')) {
    return (
      <Badge
        variant='destructive'
        className='bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20 font-medium'
      >
        ● {translate('Quota exhausted')}
      </Badge>
    )
  }
  if (statuses.includes('expired')) {
    return (
      <Badge
        variant='warning'
        className='bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20 font-medium'
      >
        ● {translate('Expired')}
      </Badge>
    )
  }
  if (statuses.includes('disabled')) {
    return <Badge variant='secondary'>{translate('Disabled')}</Badge>
  }
  return (
    <Badge
      variant='outline'
      className='bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20 font-medium'
    >
      ● {translate('OK')}
    </Badge>
  )
}

function UsedPercentCell(props: { percentStr: string }) {
  const clean = props.percentStr.replace('%', '').trim()
  const num = parseFloat(clean)

  if (isNaN(num)) {
    return <span>{props.percentStr}</span>
  }

  const barColor =
    num >= 90
      ? 'bg-rose-500'
      : num >= 70
        ? 'bg-amber-500'
        : 'bg-emerald-500'

  return (
    <div className='flex flex-col items-start gap-1 min-w-[90px]'>
      <span className='font-medium text-xs tabular-nums'>{props.percentStr}</span>
      <div className='h-1.5 w-full bg-muted rounded-full overflow-hidden'>
        <div
          className={cn('h-full rounded-full transition-all duration-300', barColor)}
          style={{ width: `${Math.min(num, 100)}%` }}
        />
      </div>
    </div>
  )
}

export function StatCards(props: {
  report: XPikiQuotaReport
  cachedTimestamp?: number
  isPending?: boolean
}) {
  const { report, cachedTimestamp, isPending } = props
  const { t } = useTranslation()

  const activeKeysCount = report.keys.filter(
    (k) => (k.status || []).includes('ok') && !k.error
  ).length
  const totalKeys = report.keys.length
  const issueKeysCount = totalKeys - activeKeysCount
  const pools = referenceTokenPools(report)

  const formattedTime = cachedTimestamp
    ? new Date(cachedTimestamp).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      })
    : null

  const formattedDate = cachedTimestamp
    ? new Date(cachedTimestamp).toLocaleDateString()
    : null

  return (
    <div className='grid grid-cols-1 md:grid-cols-3 gap-4'>
      <Card className='relative overflow-hidden rounded-xl border border-border bg-background shadow-none ring-0'>
        <CardHeader className='pb-2'>
          <div className='flex items-center justify-between'>
            <CardDescription className='font-medium'>{t('Token quy đổi còn lại')}</CardDescription>
            <div className='flex items-center gap-1.5'>
              <div className='p-2 rounded-lg bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                <Coins className='size-4' />
              </div>
            </div>
          </div>
          <CardTitle className='text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400'>
            {pools.length ? pools.map((pool) => (
              <span key={pool.model} className='block text-base'>
                {pool.label}: {pool.unlimited ? t('Unlimited') : `${formatPoolTokens(pool.remaining)} token`}
              </span>
            )) : '—'}
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 space-y-2'>
          <div className='text-xs text-muted-foreground'>
            {t('GPT tính theo token Astra; Claude tính theo token Opus 5.5. Không cộng lẫn hai loại token.')}
          </div>
        </CardContent>
      </Card>

      <Card className='relative overflow-hidden rounded-xl border border-border bg-background shadow-none ring-0'>
        <CardHeader className='pb-2'>
          <div className='flex items-center justify-between'>
            <CardDescription className='font-medium'>{t('Active Keys')}</CardDescription>
            <div className='p-2 rounded-lg bg-blue-500/10 text-blue-600 dark:text-blue-400'>
              <KeyRound className='size-4' />
            </div>
          </div>
          <CardTitle className='text-2xl font-bold tracking-tight'>
            {activeKeysCount} / {totalKeys}
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 space-y-2'>
          <div className='flex items-center gap-1.5 text-xs'>
            {issueKeysCount > 0 ? (
              <Badge variant='destructive' className='text-[10px] h-4 py-0 px-1.5'>
                {issueKeysCount} {t('Issues detected')}
              </Badge>
            ) : (
              <Badge variant='outline' className='text-[10px] h-4 py-0 px-1.5 border-emerald-500/30 text-emerald-600 dark:text-emerald-400'>
                {t('All keys active')}
              </Badge>
            )}
          </div>
          <div className='text-[11px] text-muted-foreground pt-1.5'>
            {t('Số khóa kết nối sẵn sàng định tuyến')}
          </div>
        </CardContent>
      </Card>

      <Card className='relative overflow-hidden rounded-xl border border-border bg-background shadow-none ring-0'>
        <CardHeader className='pb-2'>
          <div className='flex items-center justify-between'>
            <CardDescription className='font-medium'>{t('Last updated')}</CardDescription>
            <div className='p-2 rounded-lg bg-purple-500/10 text-purple-600 dark:text-purple-400'>
              <Clock className='size-4' />
            </div>
          </div>
          <CardTitle className='text-2xl font-bold tracking-tight'>
            {isPending ? t('Updating...') : (formattedTime || '—')}
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 space-y-2'>
          <div className='text-xs text-muted-foreground flex items-center justify-between'>
            <span>{formattedDate || '—'}</span>
            {cachedTimestamp && !isPending && (
              <Badge variant='secondary' className='text-[10px] h-4 py-0 px-1.5'>
                {t('Cached data')}
              </Badge>
            )}
          </div>
          <div className='text-[11px] text-muted-foreground pt-1.5'>
            {t('Dữ liệu đồng bộ trực tiếp từ upstream')}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

export function QuotaTables(props: { report: XPikiQuotaReport; customKeys: CustomKeyItem[] }) {
  const { t } = useTranslation()
  const rows = quotaRows(props.report, t)

  const [channelStatuses, setChannelStatuses] = useState<Record<number, number>>({})
  const [updatingChannelId, setUpdatingChannelId] = useState<number | null>(null)

  const handleToggleChannel = async (channelId: number, targetStatus: number) => {
    setUpdatingChannelId(channelId)
    try {
      await updateChannelStatus(channelId, targetStatus)
      setChannelStatuses((prev) => ({ ...prev, [channelId]: targetStatus }))
      toast.success(t('Channel status updated'))
    } catch (error) {
      handleServerError(error, t('Channel update failed'))
    } finally {
      setUpdatingChannelId(null)
    }
  }

  return (
    <div className='space-y-6'>
      <Card className='border-border shadow-xs'>
        <CardHeader className='border-b py-3 px-4 flex flex-row items-center justify-between'>
          <div>
            <CardTitle className='text-base font-semibold'>{t('Keys Overview')}</CardTitle>
            <CardDescription className='text-xs'>
              {t('Total {{count}} records', { count: props.report.keys.length })}
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent className='p-0 overflow-x-auto'>
          <StaticDataTable
            data={rows}
            getRowKey={(row) => row.id}
            getRowClassName={(row) =>
              row.isTotal ? 'bg-muted/70 font-semibold border-t-2 border-border' : undefined
            }
            columns={[
              {
                id: 'key',
                header: t('Key'),
                cell: (row) => (
                  <div className='flex items-center gap-2 font-medium'>
                    <span>{row.key}</span>
                  </div>
                ),
              },
              {
                id: 'channel',
                header: t('Channel'),
                cell: (row) => {
                  if (row.isTotal) return null
                  const chId = row.rawKey.channel_id
                  const chName = row.rawKey.channel_name
                  const chStatus =
                    channelStatuses[chId ?? 0] ?? row.rawKey.channel_status

                  if (!chId) {
                    return (
                      <Badge variant='outline' className='text-[10px] text-muted-foreground'>
                        {t('Custom Key')}
                      </Badge>
                    )
                  }

                  const isEnabled = chStatus === 1
                  return (
                    <div className='flex items-center gap-2'>
                      <a
                        href='/channels'
                        className='inline-flex items-center gap-1 font-medium hover:underline text-primary text-xs'
                        title={t('View in Channels')}
                      >
                        <Radio className='size-3 text-muted-foreground' />
                        <span>#{chId} {chName}</span>
                        <ExternalLink className='size-2.5 text-muted-foreground' />
                      </a>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        className='h-5 px-1.5 text-[11px]'
                        disabled={updatingChannelId === chId}
                        onClick={() =>
                          handleToggleChannel(chId, isEnabled ? 2 : 1)
                        }
                        title={
                          isEnabled
                            ? t('Click to disable channel')
                            : t('Click to enable channel')
                        }
                      >
                        {isEnabled ? (
                          <Badge
                            variant='outline'
                            className='bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/30 text-[10px] h-4 py-0 cursor-pointer'
                          >
                            ● {t('Enabled')}
                          </Badge>
                        ) : (
                          <Badge
                            variant='secondary'
                            className='text-[10px] h-4 py-0 cursor-pointer text-muted-foreground'
                          >
                            ○ {t('Disabled')}
                          </Badge>
                        )}
                      </Button>
                    </div>
                  )
                },
              },
              {
                id: 'used',
                header: t('Used amount'),
                cell: (row) => (
                  <span
                    className={
                      row.isTotal
                        ? 'font-bold text-foreground tabular-nums'
                        : 'font-semibold text-foreground tabular-nums'
                    }
                  >
                    {row.usedAstra}
                  </span>
                ),
                cellClassName: 'tabular-nums',
              },
              {
                id: 'limit',
                header: t('Limit'),
                cell: (row) => (
                  <span
                    className={
                      row.isTotal
                        ? 'font-bold text-foreground tabular-nums'
                        : 'font-semibold text-foreground tabular-nums'
                    }
                  >
                    {row.rawKey.unlimited
                      ? t('Unlimited')
                      : row.limitAstra}
                  </span>
                ),
                cellClassName: 'tabular-nums',
              },
              {
                id: 'remaining',
                header: t('Remaining'),
                cell: (row) => (
                  <span
                    className={
                      row.isTotal
                        ? 'font-bold text-base text-emerald-600 dark:text-emerald-400 tabular-nums'
                        : 'font-semibold text-emerald-600 dark:text-emerald-400 tabular-nums'
                    }
                  >
                    {row.rawKey?.unlimited
                      ? t('Unlimited')
                      : row.remainingAstra}
                  </span>
                ),
                cellClassName: 'tabular-nums',
              },
              {
                id: 'percent',
                header: t('Used percentage'),
                cell: (row) => (
                  <UsedPercentCell percentStr={row.usedPercent} />
                ),
              },
              {
                id: 'status',
                header: t('Status'),
                cell: (row) =>
                  row.isTotal ? null : (
                    <StatusBadge keyResult={row.rawKey} translate={t} />
                  ),
              },
            ]}
          />
        </CardContent>
      </Card>

      {props.report.total_incomplete && (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'Some keys could not be checked; the total only includes successful keys.'
            )}
          </AlertDescription>
        </Alert>
      )}

      <ModelRatioTable profiles={props.report.model_profiles} customKeys={props.customKeys} />

      {props.report.no_model_price_keys.length > 0 && (
        <Alert>
          <AlertDescription>
            {t('Model pricing is unavailable for: {{keys}}', {
              keys: props.report.no_model_price_keys.join(', '),
            })}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}

function ModelRatioTable(props: { profiles: XPikiQuotaModelProfile[]; customKeys: CustomKeyItem[] }) {
  const { t } = useTranslation()
  const [isExpanded, setIsExpanded] = useState(false)
  const [profileFilter, setProfileFilter] = useState('')
  const pricingQuery = useQuery({
    queryKey: ['xpiki-observed-tariffs', props.customKeys],
    queryFn: () => checkXPikiQuota(props.customKeys, true),
    enabled: isExpanded,
    staleTime: 10 * 60 * 1000,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const profiles = pricingQuery.data?.model_profiles ?? props.profiles
  const profile = profiles.find((item) => item.labels.join('|') === profileFilter) ?? profiles[0]

  return (
    <Card className='rounded-2xl border-[#26282d] bg-[#17181b] p-2 shadow-none'>
      <CardHeader
        className={cn(
          'py-3.5 px-4 cursor-pointer select-none transition-colors hover:bg-muted/30',
          isExpanded && 'border-b'
        )}
        onClick={() => setIsExpanded((prev) => !prev)}
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <CardTitle className='text-sm font-semibold flex items-center gap-2'>
            <Sparkles className='size-4 text-amber-500' />
            <span>{t('Giá theo token đã đối soát')}</span>
          </CardTitle>
          <div className='flex items-center gap-2'>
            <Badge variant='outline' className='text-xs'>{profiles.reduce((count, item) => count + item.labels.length, 0)} key</Badge>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='h-7 px-2.5 text-xs gap-1'
              onClick={(e) => {
                e.stopPropagation()
                setIsExpanded((prev) => !prev)
              }}
            >
              <span>{isExpanded ? t('Hide') : t('Show')}</span>
              {isExpanded ? (
                <ChevronUp className='size-3.5' />
              ) : (
                <ChevronDown className='size-3.5' />
              )}
            </Button>
          </div>
        </div>
      </CardHeader>
      {isExpanded && (
        <CardContent className='p-0 overflow-x-auto'>
          <div className='flex flex-wrap items-center gap-3 px-4 pt-3'>
            <Select value={profile?.labels.join('|') ?? ''} onValueChange={(value) => {
              if (typeof value === 'string') setProfileFilter(value)
            }}>
              <SelectTrigger size='sm' aria-label={t('Key đối soát giá')}>
                <SelectValue>{profile?.labels.join(', ') || t('Chưa có key')}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {profiles.map((item) => (
                  <SelectItem key={item.labels.join('|')} value={item.labels.join('|')}>
                    {item.labels.join(', ')}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button variant='outline' size='sm' disabled={pricingQuery.isFetching} onClick={() => pricingQuery.refetch()}>
              <RefreshCw className={cn('size-3.5', pricingQuery.isFetching && 'animate-spin')} />
              {t('Đối soát lại giá')}
            </Button>
            {pricingQuery.isFetching && <span className='text-xs text-muted-foreground'>{t('Đang tải biên lai đối soát giá…')}</span>}
          </div>
          {pricingQuery.isError && <ErrorState title={t('Không tải được giá đối soát')} onRetry={() => pricingQuery.refetch()} />}
          <p className='px-4 py-3 text-xs text-muted-foreground'>
            {profile?.price_basis}. {t('Mức thu thực tế /1M token, đã gồm hệ số và khớp ít nhất 3 biên lai cùng key/model/mức thu. Quét tối đa 1.000 biên lai gần nhất/key. Model có nhiều mức thu được tách riêng; đây không phải giá cơ sở hay cam kết giá tương lai.')}
            <span className='block mt-1'>{t('Input là token không cache; cache-read tính riêng, không trừ hai lần. Cache-write và model chưa đối soát để trống, không phải miễn phí.')}</span>
            {profile?.pricing_checked_at && <span className='block mt-1'>{t('Lần tải biên lai')}: {new Date(profile.pricing_checked_at).toLocaleString('vi-VN')} · {t('Kết quả được lưu tối đa 10 phút để tránh giới hạn tải XPiki.')}</span>}
            {profile?.pricing_warning && <span className='block mt-1 text-amber-600'>{profile.pricing_warning}</span>}
          </p>
          <StaticDataTable
            data={actualTariffRows(profile?.models ?? [])}
            getRowKey={(row) => `${row.model_slug}-${row.observed_multiplier || 'unknown'}`}
            columns={[
              {
                id: 'model',
                header: t('Model'),
                cell: (row) => (
                  <div className='font-semibold text-foreground'>
                    {row.model}
                    {!row.observed_multiplier && row.unverified_samples > 0 && <div className='text-xs font-normal text-amber-600'>{row.unverified_samples} biên lai chưa đủ đối soát mức thu</div>}
                  </div>
                ),
              },
              {
                id: 'price',
                header: t('credit / 1M input thực'),
                cell: (row) => row.effective_input_vnd_per_1m ? formatXPikiVND(row.effective_input_vnd_per_1m, false) : t('Chưa xác minh'),
                cellClassName: 'tabular-nums',
              },
              {
                id: 'output_price',
                header: t('credit / 1M output thực'),
                cell: (row) => row.effective_output_vnd_per_1m ? formatXPikiVND(row.effective_output_vnd_per_1m, false) : t('Chưa xác minh'),
                cellClassName: 'tabular-nums',
              },
              {
                id: 'multiplier',
                header: t('Hệ số đã bao gồm'),
                cell: (row) => row.observed_multiplier ? `×${row.observed_multiplier}` : '—',
                cellClassName: 'tabular-nums font-medium',
              },
              {
                id: 'cache_read_price', header: t('credit / 1M cache-read token'),
                cell: (row) => row.effective_cache_read_vnd_per_1m ? formatXPikiVND(row.effective_cache_read_vnd_per_1m, false) : t('Chưa xác minh'), cellClassName: 'tabular-nums',
              },
              {
                id: 'cache_write_price', header: t('credit / 1M cache-write token'),
                cell: (row) => row.cache_write_vnd_per_1m ? formatXPikiVND(row.cache_write_vnd_per_1m, false) : t('Chưa xác định'), cellClassName: 'tabular-nums',
              },
              {
                id: 'verification', header: t('Mẫu đối soát'),
                cell: (row) => row.verified_samples > 0 ? (
                  <span title={row.verified_at}>{row.verified_samples} request · {row.verified_at ? new Date(row.verified_at).toLocaleString('vi-VN') : ''}</span>
                ) : '—',
              },
            ]}
          />
        </CardContent>
      )}
    </Card>
  )
}

function ManageKeysDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelKeys: XPikiQuotaKeyResult[]
  customKeys: CustomKeyItem[]
  onSaveKeys: (keys: CustomKeyItem[]) => void
  onSaveAndCheck: (keys: CustomKeyItem[]) => void
}) {
  const {
    open,
    onOpenChange,
    channelKeys,
    customKeys,
    onSaveKeys,
    onSaveAndCheck,
  } = props
  const { t } = useTranslation()

  const [activeTab, setActiveTab] = useState<'channels' | 'single' | 'batch'>(
    'channels'
  )
  const [singleLabel, setSingleLabel] = useState('')
  const [singleKey, setSingleKey] = useState('')
  const [batchText, setBatchText] = useState('')

  const handleAddSingle = () => {
    const key = singleKey.trim()
    if (!key) {
      toast.error(t('Please enter an API key'))
      return
    }
    const newItem: CustomKeyItem = {
      id: `custom-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      label: singleLabel.trim() || `${t('Custom Key')} ${customKeys.length + 1}`,
      key,
    }
    const updated = [...customKeys, newItem]
    onSaveKeys(updated)
    setSingleLabel('')
    setSingleKey('')
    toast.success(t('Key added'))
  }

  const handleAddBatch = () => {
    const lines = batchText
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)

    if (lines.length === 0) {
      toast.error(t('Please enter an API key'))
      return
    }

    const newItems: CustomKeyItem[] = []
    lines.forEach((line, index) => {
      let label = ''
      let key = line
      if (line.includes(':') && !line.startsWith('sk-')) {
        const parts = line.split(':')
        label = parts[0].trim()
        key = parts.slice(1).join(':').trim()
      }
      if (!key) return
      newItems.push({
        id: `custom-${Date.now()}-${index}-${Math.random().toString(36).slice(2, 7)}`,
        label:
          label || `${t('Custom Key')} ${customKeys.length + newItems.length + 1}`,
        key,
      })
    })

    if (newItems.length === 0) {
      toast.error(t('Please enter an API key'))
      return
    }

    const updated = [...customKeys, ...newItems]
    onSaveKeys(updated)
    setBatchText('')
    toast.success(t('Key added') + ` (${newItems.length})`)
  }

  const handleDeleteKey = (id?: string) => {
    const updated = customKeys.filter((k) => k.id !== id)
    onSaveKeys(updated)
  }

  const handleDeleteAll = () => {
    onSaveKeys([])
  }

  const handleSaveAndCheckFinal = () => {
    let updated = [...customKeys]
    if (activeTab === 'single' && singleKey.trim()) {
      const newItem: CustomKeyItem = {
        id: `custom-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        label: singleLabel.trim() || `${t('Custom Key')} ${updated.length + 1}`,
        key: singleKey.trim(),
      }
      updated = [...updated, newItem]
      setSingleKey('')
      setSingleLabel('')
    } else if (activeTab === 'batch' && batchText.trim()) {
      const lines = batchText
        .split('\n')
        .map((l) => l.trim())
        .filter(Boolean)
      const newItems: CustomKeyItem[] = []
      lines.forEach((line, index) => {
        let label = ''
        let key = line
        if (line.includes(':') && !line.startsWith('sk-')) {
          const parts = line.split(':')
          label = parts[0].trim()
          key = parts.slice(1).join(':').trim()
        }
        if (!key) return
        newItems.push({
          id: `custom-${Date.now()}-${index}-${Math.random().toString(36).slice(2, 7)}`,
          label:
            label || `${t('Custom Key')} ${updated.length + newItems.length + 1}`,
          key,
        })
      })
      if (newItems.length > 0) {
        updated = [...updated, ...newItems]
        setBatchText('')
      }
    }
    onSaveKeys(updated)
    onOpenChange(false)
    onSaveAndCheck(updated)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg max-h-[90vh] flex flex-col'>
        <DialogHeader>
          <DialogTitle>{t('Manage Keys')}</DialogTitle>
          <DialogDescription>
            {t(
              'Quản lý danh sách Key hệ thống từ Kênh hoặc thêm API Key tùy chỉnh.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex gap-1.5 border-b pb-2 text-sm'>
          <button
            type='button'
            className={cn(
              'px-3 py-1.5 rounded-md font-medium text-xs transition-colors flex items-center gap-1.5',
              activeTab === 'channels'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted'
            )}
            onClick={() => setActiveTab('channels')}
          >
            <Radio className='size-3.5' />
            <span>
              {t('Channel Keys')} ({channelKeys.length})
            </span>
          </button>
          <button
            type='button'
            className={cn(
              'px-3 py-1.5 rounded-md font-medium text-xs transition-colors',
              activeTab === 'single'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted'
            )}
            onClick={() => setActiveTab('single')}
          >
            {t('Single Key')}
          </button>
          <button
            type='button'
            className={cn(
              'px-3 py-1.5 rounded-md font-medium text-xs transition-colors',
              activeTab === 'batch'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted'
            )}
            onClick={() => setActiveTab('batch')}
          >
            {t('Batch Add')}
          </button>
        </div>

        <div className='flex-1 overflow-y-auto space-y-4 py-2 pr-1'>
          {activeTab === 'channels' && (
            <div className='space-y-3'>
              <div className='bg-primary/5 border border-primary/20 rounded-md p-3 text-xs space-y-1.5 text-muted-foreground'>
                <div className='font-semibold text-foreground flex items-center gap-1.5'>
                  <Radio className='size-3.5 text-primary' />
                  <span>{t('Đồng bộ tự động với trang Kênh')}</span>
                </div>
                <p>
                  {t(
                    'Các key này được cấu hình trực tiếp từ trang Kênh (Channels). Khi bạn thêm, sửa hoặc xóa kênh/key bên Kênh, hệ thống sẽ tự động đồng bộ ngay khi bấm Kiểm tra quota.'
                  )}
                </p>
                <div className='pt-1'>
                  <a
                    href='/channels'
                    className='inline-flex items-center gap-1 text-primary hover:underline font-medium text-xs'
                  >
                    <span>{t('Đi đến trang Quản lý Kênh để thêm/xóa Key')}</span>
                    <ExternalLink className='size-3' />
                  </a>
                </div>
              </div>

              <div className='space-y-2'>
                <div className='flex items-center justify-between text-xs font-semibold text-muted-foreground'>
                  <span>{t('Danh sách Key hiện có trong Kênh')}</span>
                  <a
                    href='/channels'
                    className='text-primary hover:underline flex items-center gap-1 font-normal text-[11px]'
                  >
                    <Plus className='size-3' />
                    <span>{t('Thêm Kênh mới')}</span>
                  </a>
                </div>

                {channelKeys.length === 0 ? (
                  <p className='text-xs text-muted-foreground italic text-center py-4'>
                    {t('Chưa có kênh XPiki nào được cấu hình trong hệ thống.')}
                  </p>
                ) : (
                  <div className='space-y-1.5 max-h-56 overflow-y-auto pr-1'>
                    {channelKeys.map((item, idx) => {
                      const isEnabled = item.channel_status === 1
                      return (
                        <div
                          key={`ch-${item.channel_id}-${idx}`}
                          className='flex items-center justify-between bg-muted/40 p-2 rounded-md text-xs border border-border/50'
                        >
                          <div className='flex flex-col gap-0.5 truncate mr-2'>
                            <div className='flex items-center gap-1.5'>
                              <span className='font-semibold text-foreground'>
                                #{item.channel_id} {item.channel_name || item.label}
                              </span>
                              {isEnabled ? (
                                <Badge
                                  variant='outline'
                                  className='bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/30 text-[10px] h-4 py-0'
                                >
                                  ● {t('Enabled')}
                                </Badge>
                              ) : (
                                <Badge
                                  variant='secondary'
                                  className='text-[10px] h-4 py-0 text-muted-foreground'
                                >
                                  ○ {t('Disabled')}
                                </Badge>
                              )}
                            </div>
                            {item.name && (
                              <span className='text-muted-foreground text-[11px] truncate'>
                                {item.name}
                              </span>
                            )}
                          </div>
                          <a
                            href='/channels'
                            className='inline-flex items-center gap-1 text-[11px] text-primary hover:underline shrink-0'
                            title={t('Xem kênh bên /channels')}
                          >
                            <span>{t('Sửa')}</span>
                            <ExternalLink className='size-3' />
                          </a>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            </div>
          )}

          {activeTab === 'single' && (
            <div className='space-y-3'>
              <p className='text-xs text-muted-foreground'>
                {t(
                  'Thêm API key tùy chỉnh để theo dõi quota trực tiếp mà không cần cấu hình Kênh.'
                )}
              </p>
              <div>
                <label className='text-xs font-medium text-muted-foreground mb-1 block'>
                  {t('Key Name / Note')}
                </label>
                <Input
                  placeholder='vd: Key dự phòng...'
                  value={singleLabel}
                  onChange={(e) => setSingleLabel(e.target.value)}
                />
              </div>
              <div>
                <label className='text-xs font-medium text-muted-foreground mb-1 block'>
                  {t('API Key')} *
                </label>
                <Input
                  type='password'
                  placeholder='sk-...'
                  value={singleKey}
                  onChange={(e) => setSingleKey(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      handleAddSingle()
                    }
                  }}
                />
              </div>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='w-full'
                onClick={handleAddSingle}
              >
                <Plus className='size-3.5 mr-1' /> {t('Add Key')}
              </Button>
            </div>
          )}

          {activeTab === 'batch' && (
            <div className='space-y-3'>
              <p className='text-xs text-muted-foreground'>
                {t(
                  'Dán nhiều API key (mỗi key một dòng, hoặc Tên: sk-...).'
                )}
              </p>
              <div>
                <label className='text-xs font-medium text-muted-foreground mb-1 block'>
                  {t('Paste one key per line')}
                </label>
                <Textarea
                  rows={4}
                  placeholder={`Key 1: sk-...\nKey 2: sk-...`}
                  value={batchText}
                  onChange={(e) => setBatchText(e.target.value)}
                />
              </div>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='w-full'
                onClick={handleAddBatch}
              >
                <Plus className='size-3.5 mr-1' /> {t('Add Key')}
              </Button>
            </div>
          )}

          {/* Custom keys list */}
          <div className='border-t pt-3'>
            <div className='flex items-center justify-between mb-2'>
              <span className='text-xs font-semibold text-muted-foreground'>
                {t('Custom Keys')} ({customKeys.length})
              </span>
              {customKeys.length > 0 && (
                <button
                  type='button'
                  onClick={handleDeleteAll}
                  className='text-[11px] text-destructive hover:underline'
                >
                  {t('Delete all')}
                </button>
              )}
            </div>

            {customKeys.length === 0 ? (
              <p className='text-xs text-muted-foreground italic text-center py-2'>
                {t('No custom keys added yet')}
              </p>
            ) : (
              <div className='space-y-1.5 max-h-36 overflow-y-auto pr-1'>
                {customKeys.map((item, idx) => (
                  <div
                    key={item.id || idx}
                    className='flex items-center justify-between bg-muted/40 px-2.5 py-1.5 rounded-md text-xs border border-border/50'
                  >
                    <div className='truncate mr-2'>
                      <span className='font-medium'>{item.label || `Key ${idx + 1}`}</span>
                      <span className='ml-2 text-muted-foreground font-mono text-[11px]'>
                        {item.key.slice(0, 7)}...{item.key.slice(-4)}
                      </span>
                    </div>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='size-6 p-0 text-muted-foreground hover:text-destructive'
                      onClick={() => handleDeleteKey(item.id)}
                    >
                      <Trash2 className='size-3.5' />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <DialogFooter className='border-t pt-3 gap-2 sm:gap-0'>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Close')}
          </Button>
          <Button
            type='button'
            onClick={handleSaveAndCheckFinal}
          >
            <RefreshCw className='size-3.5 mr-1.5' />
            {t('Save & Check')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function XPikiLogsCard(props: { customKeys: CustomKeyItem[] }) {
  const { t } = useTranslation()
  const [preferences, setPreferences] = useState<{
    isExpanded: boolean
    pageSize: number
  }>(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(STORAGE_KEY_LOGS) || '{}')
      return {
        isExpanded: saved?.isExpanded === true,
        pageSize: [20, 50, 100].includes(saved?.pageSize) ? saved.pageSize : 20,
      }
    } catch {
      return { isExpanded: false, pageSize: 20 }
    }
  })
  const { isExpanded, pageSize } = preferences
  const setIsExpanded = (update: (previous: boolean) => boolean) =>
    setPreferences((previous) => ({
      ...previous,
      isExpanded: update(previous.isExpanded),
    }))
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY_LOGS, JSON.stringify(preferences))
    } catch {
      // Storage may be unavailable; filters still work for this session.
    }
  }, [preferences])
  const [keyFilter, setKeyFilter] = useState('all')
  const [channelFilter, setChannelFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [modelFilter, setModelFilter] = useState('all')
  const [userFilter, setUserFilter] = useState<string[]>([])
  const [dateFilter, setDateFilter] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [page, setPage] = useState(1)
  // Query data becomes undefined on a new filter. Keep selector options, not
  // the previous request rows, available while the new response is pending.
  const [keyOptions, setKeyOptions] = useState<XPikiKeyOption[]>([])
  const [modelOptions, setModelOptions] = useState<string[]>([])
  const [userOptions, setUserOptions] = useState<string[]>([])

  const formatDateToYYYYMMDD = (d: Date) => {
    const year = d.getFullYear()
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    return `${year}-${month}-${day}`
  }
  const todayStr = useMemo(() => formatDateToYYYYMMDD(new Date()), [])

  const {
    data: logsData,
    error: logsError,
    isLoading: isPending,
    isFetching,
    refetch,
  } = useQuery({
    queryKey: [
      'newapi-customer-request-logs',
      props.customKeys,
      keyFilter,
      channelFilter,
      statusFilter,
      modelFilter,
      userFilter,
      dateFilter,
      searchQuery,
      page,
      pageSize,
    ],
    queryFn: ({ signal }) =>
      fetchXPikiLogs({
        custom_keys: props.customKeys.map((k) => ({
          label: k.label,
          key: k.key,
        })),
        key_filter: keyFilter === 'all' ? undefined : keyFilter,
        channel_filter: channelFilter === 'all' ? undefined : channelFilter,
        status: statusFilter === 'all' ? undefined : statusFilter,
        model: modelFilter === 'all' ? undefined : modelFilter,
        user: userFilter.length ? userFilter.join(',') : undefined,
        date: dateFilter.trim() || undefined,
        search: searchQuery.trim() || undefined,
        page,
        page_size: pageSize,
        source: 'newapi',
        group_mode: 'grouped',
      }, signal),
    // Poll the local customer-request view only while the log card is open.
    enabled: isExpanded,
    retry: false,
    refetchInterval: isExpanded ? 10000 : false,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    refetchOnMount: true,
  })

  useEffect(() => {
    if (logsData) {
      setKeyOptions(logsData.key_options ?? [])
      setModelOptions(logsData.model_options ?? [])
      setUserOptions(logsData.user_options ?? [])
    }
  }, [logsData])

  const selectedKeyLabel =
    keyFilter === 'all'
      ? t('Tất cả khóa API')
      : keyOptions.find((o) => o.value === keyFilter)?.label ||
        keyFilter

  const statusLabelMap: Record<string, string> = {
    all: t('All Statuses'),
    success: t('Success'),
    retry_recovered: t('Fallback successful'),
    failed: t('Failed'),
    canceled: t('Canceled'),
  }

  const columns: StaticDataTableColumn<XPikiLogItem>[] = [
    {
      id: 'time',
      header: t('Time'),
      cell: (row) => {
        const d = new Date(row.created_at)
        const isValid = !isNaN(d.getTime())
        const timeStr = isValid
          ? d.toLocaleTimeString([], {
              hour: '2-digit',
              minute: '2-digit',
              second: '2-digit',
            })
          : row.created_at
        const dateStr = isValid
          ? d.toLocaleDateString([], {
              day: '2-digit',
              month: '2-digit',
              year: 'numeric',
            })
          : ''
        return (
          <div className='flex flex-col text-xs'>
            <span className='font-medium text-foreground tabular-nums'>
              {timeStr}
            </span>
            <span className='text-[11px] text-muted-foreground tabular-nums'>
              {dateStr}
            </span>
          </div>
        )
      },
      cellClassName: 'whitespace-nowrap',
    },
    {
      id: 'token_name',
      header: t('Requesting Key'),
      cell: (row) => (
        <Badge
          variant='secondary'
          className='text-xs font-semibold px-2 py-0.5 bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20'
        >
          <KeyRound className='size-3 mr-1 inline' />
          {row.token_id ? `#${row.token_id} · ${row.token_name || t('Khóa không đặt tên')}` : row.token_name || t('Log cũ không lưu ID khóa')}
        </Badge>
      ),
    },
    {
      id: 'channel',
      header: t('Target Channel'),
      cell: (row) => (
        <div className='flex flex-col items-start gap-0.5'>
          {row.channel_id ? (
            <Badge
              variant='outline'
              className='text-xs font-semibold border-primary/30 text-primary bg-primary/5 px-2 py-0.5'
            >
              #{row.channel_id} {row.channel_name || 'Channel'}
            </Badge>
          ) : (
            <Badge variant='outline' className='text-xs font-normal px-2 py-0.5'>
              {row.channel_name || '—'}
            </Badge>
          )}
        </div>
      ),
    },

    {
      id: 'model',
      header: t('Model'),
      cell: (row) => (
        <div className='flex flex-col items-start gap-0.5'>
          <span className='font-semibold text-xs text-foreground font-mono'>
            {row.model_slug}
          </span>
        </div>
      ),
    },
    {
      id: 'username',
      header: t('User'),
      cell: (row) => <span className='text-xs font-medium'>{row.username || t('Không xác định')}</span>,
    },
    {
      id: 'status',
      header: t('Status / Fallback'),
      cell: (row) => {
        const isSuccess = row.request_outcome
          ? ['success', 'retry_recovered'].includes(row.request_outcome)
          : row.status === 'success'
        const isFailed = row.request_outcome
          ? ['failed_final', 'stream_failed'].includes(row.request_outcome)
          : row.status === 'failed'
        const isCanceled = row.request_outcome
          ? row.request_outcome === 'cancelled'
          : row.status === 'canceled'
        const isIncomplete = row.request_outcome === 'stream_incomplete'
        const isUnknown = row.request_outcome === 'unknown'
        const isRetried = Boolean(row.retried)
        const fallbackChannel =
          row.fallback_channel_name ||
          (row.fallback_channel_id ? `#${row.fallback_channel_id}` : undefined)
        const originChannel =
          row.initial_channel_name ||
          row.channel_name ||
          (row.channel_id ? `#${row.channel_id}` : undefined)
        const fallbackRoute =
          (row.channel_trail?.length || 0) > 1
            ? row.channel_trail?.join(' → ')
            : fallbackChannel
              ? [originChannel, fallbackChannel].filter(Boolean).join(' → ')
              : undefined

        const formatFallbackReason = (err?: string) => {
          if (!err) return t('Connection error')
          if (err.includes('did not produce its first answer')) {
            return t('Timed out waiting for first response')
          }
          if (err.includes('context canceled')) {
            return t('Connection interrupted (Context canceled)')
          }
          if (err.includes('401') || err.includes('Unauthorized')) {
            return t('Error 401: Key authentication failed')
          }
          if (err.includes('402')) {
            return t('Error 402: Upstream quota exhausted')
          }
          if (/408|504|524/.test(err)) {
            return t('Error 408/504/524: Upstream timeout')
          }
          if (err.includes('429') || err.includes('rate limit')) {
            return t('Error 429: Rate limit exceeded')
          }
          if (err.includes('502') || err.includes('503') || err.includes('bad_response')) {
            return t('Error 502/503: Gateway error')
          }
          return err.length > 45 ? err.slice(0, 45) + '...' : err
        }

        return (
          <div className='flex flex-col items-start gap-1'>
            <div className='flex items-center gap-1.5 flex-wrap'>
              <Badge
                variant={
                  isSuccess
                    ? 'outline'
                    : isFailed
                      ? 'destructive'
                      : isIncomplete || isCanceled
                        ? 'secondary'
                        : 'secondary'
                }
                className={cn(
                  'text-xs capitalize font-semibold',
                  isSuccess &&
                    'border-emerald-500/40 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10',
                  isIncomplete &&
                    'border-amber-500/40 text-amber-600 dark:text-amber-400 bg-amber-500/10',
                  isCanceled &&
                    'border-slate-500/40 text-slate-600 dark:text-slate-400 bg-slate-500/10'
                )}
              >
                {isSuccess
                  ? t('Success')
                  : isFailed
                    ? t('Failed')
                    : isCanceled
                      ? t('Canceled')
                      : isIncomplete
                        ? t('Incomplete')
                        : isUnknown
                          ? t('Insufficient terminal evidence')
                          : row.status}
              </Badge>
            </div>
            {fallbackRoute && (
              <div className='flex flex-col items-start gap-0.5'>
              {isRetried && (isSuccess || isFailed) && (
                <span className={cn('text-[11px] font-medium', isSuccess ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive')}>
                  {isSuccess ? t('Fallback successful') : t('Fallback exhausted')}
                </span>
              )}
              <span
                className='text-[11px] text-amber-700 dark:text-amber-300 max-w-[420px]'
                title={fallbackRoute}
              >
                {t('Fallback')}: {fallbackRoute}
              </span>
              </div>
            )}
            {row.attempt_number !== undefined && (
              <span className='text-[11px] text-muted-foreground'>
                {t('Attempt {{attempt}}/{{total}}', {
                  attempt: row.attempt_number,
                  total: row.total_attempts,
                })}
              </span>
            )}
            {isRetried && row.initial_error_code && (
              <div
                className='flex items-center gap-1 text-[11px] text-amber-700 dark:text-amber-300 bg-amber-500/10 border border-amber-500/20 rounded px-1.5 py-0.5 font-medium max-w-[360px]'
                title={t('Full original error: {{error}}', { error: row.initial_error_code })}
              >
                <span className='text-red-500 font-bold shrink-0'>
                  {t('{{channel}} error:', { channel: row.initial_channel_name || t('Unassigned channel') })}
                </span>
                <span className='truncate'>{formatFallbackReason(row.initial_error_code)}</span>
              </div>
            )}
            {row.error_code && (!isRetried || row.error_code !== row.initial_error_code) && (
              <span
                className='text-[11px] text-destructive font-mono max-w-[260px] truncate'
                title={row.error_code}
              >
                {row.error_code}
              </span>
            )}
          </div>
        )
      },
    },
    {
      id: 'tokens',
      header: t('Token (input / output)'),
      cell: (row) => (
        <div className='flex flex-col text-xs items-start tabular-nums'>
          <span className='font-medium text-foreground'>
            {row.input_tokens.toLocaleString()} / {row.output_tokens.toLocaleString()}
          </span>
          {row.cache_read_tokens > 0 && (
            <span className='text-[11px] text-amber-600 dark:text-amber-400'>
              Cache-read: {row.cache_read_tokens.toLocaleString()}
            </span>
          )}
          {row.cache_write_tokens > 0 && (
            <span className='text-[11px] text-amber-600 dark:text-amber-400'>Cache-write: {row.cache_write_tokens.toLocaleString()}</span>
          )}
        </div>
      ),
      cellClassName: 'tabular-nums',
    },
    {
      id: 'cost',
      header: t('Đối soát giá vốn'),
      cell: (row) => (
        <div className='flex flex-col text-xs items-start tabular-nums'>
          {row.token_cost?.status === 'reconciled' ? (
            <span className='font-semibold text-emerald-600 dark:text-emerald-400'>
              {t('Khớp token')}{row.token_cost.multiplier ? ` · ×${row.token_cost.multiplier}` : ''}
            </span>
          ) : row.token_cost?.status === 'confirmed_zero' ? (
            <span className='text-muted-foreground'>{t('XPiki không thu phí')}</span>
          ) : row.quota_source === 'not_xpiki' ? (
            <span className='text-muted-foreground text-xs'>{t('Không áp dụng')}</span>
          ) : (
            <span className='text-amber-600 text-xs' title={row.token_cost?.warning}>{t('Chưa xác minh')}</span>
          )}
        </div>
      ),
      cellClassName: 'tabular-nums',
    },
    {
      id: 'cost_vnd',
      header: t('Giá vốn (credit)'),
      cell: (row) => (
        <div className='text-xs tabular-nums min-w-[140px]'>
          <span className='font-semibold text-blue-600 dark:text-blue-400' title={row.attempts?.length ? t('Tổng giá vốn XPiki của các lượt trong request theo quy ước 1.000 VNĐ = 1 credit.') : tokenCostExplanation(row.token_cost)}>
              {formatLogCost(row.token_cost?.total_vnd)}
          </span>
          {(row.token_cost?.status === 'reconciled' || !!row.attempts?.length) && (
            <details className='mt-1 text-muted-foreground'>
              <summary className='cursor-pointer'>{t('Chi tiết theo token')}</summary>
              <div className='whitespace-pre-line mt-1 min-w-[320px]'>
                {row.attempts?.length
                  ? row.attempts.map((attempt, index) => (
                    <div key={attempt.log_id || index} className='mb-2'>
                      #{attempt.channel_id} {attempt.channel_name}: {attempt.quota_source === 'not_xpiki' ? t('Không áp dụng giá vốn XPiki') : formatLogCost(attempt.token_cost?.total_vnd)}
                      {attempt.quota_source !== 'not_xpiki' && <div>{tokenCostExplanation(attempt.token_cost)}</div>}
                    </div>
                  ))
                  : tokenCostExplanation(row.token_cost)}
              </div>
            </details>
          )}
        </div>
      ),
      cellClassName: 'tabular-nums',
    },
  ]

  return (
    <Card className='border-border shadow-xs'>
      <CardHeader
        className={cn(
          'py-4 px-6',
          isExpanded && 'border-b'
        )}
      >
        <div className='flex flex-col gap-4'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div className='flex items-center gap-2.5'>
              <div className='p-2 rounded-lg bg-primary/10 text-primary'>
                <Clock className='size-5' />
              </div>
              <div>
                <CardTitle className='text-base font-semibold'>
                  {t('Nhật ký yêu cầu NewAPI')}
                </CardTitle>
                <CardDescription className='text-xs'>
                  <span className='block font-medium'>{t('Tự tải lại mỗi 10 giây khi mở bảng log; có thể bấm Tải lại log ngay.')}</span>
                  {t('Một dòng cho mỗi request khách gửi vào NewAPI: khóa, người dùng, model, kênh xử lý cuối và chuỗi fallback. Input/output là tổng token, cache hiển thị riêng bên dưới.')}
          <span className='block'>{t('Giá vốn chỉ tính các lượt XPiki, đối soát theo token và biên lai; 9router không dùng giá XPiki. Hiển thị quy đổi 1.000 VNĐ = 1 credit, số gốc được giữ nguyên để tính tổng.')}</span>
                  {!!logsData?.quota_errors?.length && (
                    <span className='block text-amber-600'>
                      {t('Dữ liệu chưa tải/ghép đầy đủ:')}{' '}
                      {logsData.quota_errors.join('; ')}
                    </span>
                  )}
                </CardDescription>
              </div>
            </div>

            <div className='flex items-center gap-2'>
              {isExpanded && (
                <Button
                  variant='outline'
                  size='sm'
                  onClick={(e) => {
                    e.stopPropagation()
                    refetch()
                  }}
                  disabled={isFetching}
                  className='gap-1.5'
                >
                  <RefreshCw
                    className={cn('size-3.5', isFetching && 'animate-spin')}
                  />
                  <span>{t('Tải lại log')}</span>
                </Button>
              )}

              <Button
                type='button'
                variant='outline'
                size='sm'
                className='gap-1.5 text-xs'
                onClick={(e) => {
                  e.stopPropagation()
                  setIsExpanded((prev) => !prev)
                }}
              >
                <span>{isExpanded ? t('Hide') : t('Show')}</span>
                {isExpanded ? (
                  <ChevronUp className='size-4' />
                ) : (
                  <ChevronDown className='size-4' />
                )}
              </Button>
            </div>
          </div>

          {isExpanded && (
            <div
              className='flex flex-wrap items-center gap-3 pt-1'
            >
              {/* Display and pagination preferences */}
              <div className='w-full sm:w-auto min-w-[130px]'>
                <Select
                  value={String(pageSize)}
                  onValueChange={(value) => {
                    const size = Number(value)
                    if ([20, 50, 100].includes(size)) {
                      setPreferences((previous) => ({
                        ...previous,
                        pageSize: size,
                      }))
                      setPage(1)
                    }
                  }}
                >
                  <SelectTrigger
                    size='sm'
                    className='w-full'
                    aria-label={t('Số dòng/trang')}
                  >
                    <SelectValue>
                      {pageSize} {t('dòng/trang')}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {[20, 50, 100].map((size) => (
                      <SelectItem key={size} value={String(size)}>
                        {size} {t('dòng/trang')}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {/* Key Selector */}
              <div className='w-full sm:w-auto min-w-[200px]'>
                <Select
                  value={keyFilter}
                  onValueChange={(val) => {
                    if (typeof val === 'string') {
                      setKeyFilter(val)
                      setPage(1)
                    }
                  }}
                >
                  <SelectTrigger size='sm' className='w-full' aria-label={t('Lọc khóa API')}>
                    <SelectValue>{selectedKeyLabel}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value='all'>
                        {t('Tất cả khóa API')}
                      </SelectItem>
                      {keyOptions
                        .filter((opt) => opt.value.startsWith('token_id:'))
                        .map((opt) => (
                          <SelectItem key={opt.value} value={opt.value}>
                            {opt.label}
                          </SelectItem>
                        ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>

              <div className='w-full sm:w-auto min-w-[200px]'>
                <Select value={channelFilter} onValueChange={(value) => {
                  if (typeof value === 'string') { setChannelFilter(value); setPage(1) }
                }}>
                  <SelectTrigger size='sm' className='w-full' aria-label={t('Lọc kênh')}>
                    <SelectValue>{channelFilter === 'all' ? t('Tất cả kênh') : keyOptions.find((option) => option.value === channelFilter)?.label || channelFilter}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>{t('Tất cả kênh')}</SelectItem>
                    {keyOptions.filter((option) => option.value.startsWith('channel:')).map((option) => (
                      <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className='w-full sm:w-auto min-w-[200px]'>
                <MultiSelect
                  options={modelOptions.map((model) => ({ value: model, label: model }))}
                  selected={modelFilter === 'all' ? [] : modelFilter.split(',')}
                  onChange={(values) => { setModelFilter(values.length ? values.join(',') : 'all'); setPage(1) }}
                  placeholder={t('Chọn model')}
                  maxVisibleChips={2}
                />
              </div>

              <div className='w-full sm:w-auto min-w-[220px]'>
                <MultiSelect
                  options={userOptions.map((user) => ({ value: user, label: user }))}
                  selected={userFilter}
                  onChange={(values) => { setUserFilter(values); setPage(1) }}
                  placeholder={t('Chọn người dùng')}
                  maxVisibleChips={2}
                />
              </div>


              {/* Status Filter */}
              <div className='w-full sm:w-auto min-w-[130px]'>
                <Select
                  value={statusFilter}
                  onValueChange={(val) => {
                    if (typeof val === 'string') {
                      setStatusFilter(val)
                      setPage(1)
                    }
                  }}
                >
                  <SelectTrigger size='sm' className='w-full' aria-label={t('Status filter')}>
                    <SelectValue>
                      {statusLabelMap[statusFilter] || statusFilter}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value='all'>{t('All Statuses')}</SelectItem>
                      <SelectItem value='success'>{t('Success')}</SelectItem>
                      <SelectItem value='retry_recovered'>{t('Fallback successful')}</SelectItem>
                      <SelectItem value='failed'>{t('Failed')}</SelectItem>
                      <SelectItem value='canceled'>{t('Canceled')}</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>

              {/* Date Filter */}
              <div className='flex items-center gap-1.5'>
                <div className='relative'>
                  <CalendarIcon className='absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none' />
                  <Input
                    type='date'
                    value={dateFilter}
                    onChange={(e) => {
                      setDateFilter(e.target.value)
                      setPage(1)
                    }}
                    className='h-8 pl-8 pr-2 text-xs w-[140px]'
                    title={t('Filter by date')}
                  />
                </div>
                {dateFilter && (
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-7 text-muted-foreground hover:text-foreground'
                    onClick={() => {
                      setDateFilter('')
                      setPage(1)
                    }}
                    title={t('Clear')}
                  >
                    <X className='size-3.5' />
                  </Button>
                )}
                <Button
                  type='button'
                  variant={dateFilter === todayStr ? 'secondary' : 'outline'}
                  size='sm'
                  className='h-8 px-2.5 text-xs'
                  onClick={() => {
                    setDateFilter(dateFilter === todayStr ? '' : todayStr)
                    setPage(1)
                  }}
                  title={t('Today')}
                >
                  {t('Today')}
                </Button>
              </div>

              {/* Search Input */}
              <form
                onSubmit={(e) => {
                  e.preventDefault()
                  setSearchQuery(searchInput)
                  setPage(1)
                }}
                className='flex-1 min-w-[220px] flex items-center gap-1.5'
              >
                <div className='relative flex-1'>
                  <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none' />
                  <Input
                    type='text'
                    value={searchInput}
                    onChange={(e) => setSearchInput(e.target.value)}
                    placeholder={t('Request ID, model, token, người dùng, nội dung lỗi...')}
                    className='h-8 pl-8 text-xs'
                  />
                </div>
                <Button
                  type='submit'
                  variant='secondary'
                  size='sm'
                  className='h-8 px-3 text-xs'
                >
                  {t('Search')}
                </Button>
                {searchQuery && (
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-8 px-2 text-xs'
                    onClick={() => {
                      setSearchInput('')
                      setSearchQuery('')
                      setPage(1)
                    }}
                  >
                    {t('Clear')}
                  </Button>
                )}
              </form>
            </div>
          )}
        </div>
      </CardHeader>

      {isExpanded && (
        <>
          {logsData?.token_cost_summary && (
            <div className='px-4 py-3 text-xs border-b space-y-1'>
              <div>{t('Tổng giá vốn đã đối soát (trang này)')}: <strong>{formatLogCost(logsData.token_cost_summary.total_vnd)}</strong> · {logsData.token_cost_summary.known_requests} request</div>
              {logsData.token_cost_summary.unknown_requests > 0 && <div className='text-amber-600'>{logsData.token_cost_summary.unknown_requests} {t('request chưa xác minh, chưa được cộng; tổng chưa đầy đủ.')}</div>}
              <div className='text-muted-foreground'>{t('Lời/lỗ = doanh thu thực nhận từ trang bán − giá vốn của tất cả lượt gọi. Chưa có doanh thu thực nhận tại đây; tiền trừ ví NewAPI không phải doanh thu bán.')}</div>
            </div>
          )}
          {logsError && (
            <div className='px-4 py-3'>
              <ErrorState
                title={t('Không tải được nhật ký yêu cầu')}
                onRetry={() => refetch()}
              />
            </div>
          )}
          <CardContent className='p-0 overflow-x-auto'>
            <StaticDataTable
              data={logsData?.items || []}
              getRowKey={(row) =>
                row.log_key ||
                (row.log_id !== undefined
                  ? String(row.log_id)
                  : `${row.source_key_id}-${row.request_id}-${row.created_at}`)
              }
              columns={columns}
              empty={
                !logsError &&
                !isPending &&
                (!logsData?.items || logsData.items.length === 0)
              }
              emptyContent={
                <div className='py-12 text-center text-muted-foreground text-sm'>
                  {!logsData
                    ? t('Đang tải log cho điều kiện này…')
                    : logsData.incomplete
                    ? t('Chưa đối soát đủ giá vốn; dữ liệu request lấy từ NewAPI.')
                    : t('No request logs found')}
                </div>
              }
            />
          </CardContent>

          <div className='border-t py-3 px-4 flex flex-wrap items-center justify-between gap-3'>
            <div className='text-xs text-muted-foreground'>
              {logsData?.total !== undefined && logsData.total >= 0 ? (
                <span>
                  {t('Tổng {{count}} request khách', { count: logsData.total })}
                </span>
              ) : (
                <span>{t('Page {{page}}', { page })}</span>
              )}
            </div>

            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                disabled={page <= 1 || isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className='h-8 px-2.5 text-xs gap-1'
              >
                <ChevronLeft className='size-3.5' />
                <span>{t('Previous')}</span>
              </Button>
              <span className='text-xs font-medium tabular-nums px-1'>
                {page}
              </span>
              <Button
                variant='outline'
                size='sm'
                disabled={!logsData?.has_more || isFetching}
                onClick={() => setPage((p) => p + 1)}
                className='h-8 px-2.5 text-xs gap-1'
              >
                <span>{t('Next')}</span>
                <ChevronRight className='size-3.5' />
              </Button>
            </div>
          </div>
        </>
      )}
    </Card>
  )
}

export function XPikiQuotaPage() {
  const { t } = useTranslation()

  // Load custom keys from localStorage
  const [customKeys, setCustomKeys] = useState<CustomKeyItem[]>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_CUSTOM_KEYS)
      if (saved) return JSON.parse(saved)
    } catch {
      // ignore
    }
    return []
  })

  // Load cached report immediately so the user sees results without delay
  const [cachedData, setCachedData] = useState<CachedQuotaData | null>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_CACHE)
      if (saved) return JSON.parse(saved)
    } catch {
      // ignore
    }
    return null
  })

  const [manageKeysOpen, setManageKeysOpen] = useState(false)
  const [kiroCachedAt, setKiroCachedAt] = useState<number | undefined>()
  const [kiroReport, setKiroReport] = useState<KiroQuotaReport | null>(() => {
    try {
      const saved = localStorage.getItem('newapi_kiro_quota_cache_v1')
      if (!saved) return null
      const parsed = JSON.parse(saved) as { report?: KiroQuotaReport; timestamp?: number }
      if (parsed.timestamp) setTimeout(() => setKiroCachedAt(parsed.timestamp), 0)
      return parsed.report ?? null
    } catch {
      return null
    }
  })
  const [checkSignal, setCheckSignal] = useState(0)
  const [tokenCustomKeys, setTokenCustomKeys] = useState<CustomKeyItem[]>([])
  const checkTokenKey = (key: string) => {
    if (key) setTokenCustomKeys((keys) => keys.some((item) => item.key === key) ? keys : [...keys, { key, label: `Kiro Token ${keys.length + 1}` }])
    setCheckSignal((value) => value + 1)
  }

  const mutation = useMutation({
    mutationFn: (keys?: CustomKeyItem[]) => {
      const payload = (keys ?? customKeys).map((k) => ({
        label: k.label,
        key: k.key,
      }))
      return checkXPikiQuota(payload.length > 0 ? payload : undefined)
    },
    retry: false,
    onSuccess: (data) => {
      const cacheObj: CachedQuotaData = {
        report: data,
        timestamp: Date.now(),
      }
      setCachedData(cacheObj)
      try {
        localStorage.setItem(STORAGE_KEY_CACHE, JSON.stringify(cacheObj))
      } catch {
        // ignore
      }
      toast.success(t('Quota updated successfully'))
    },
    onError: (error: unknown) =>
      handleServerError(error, t('Quota check failed')),
  })

  const handleSaveKeys = (keys: CustomKeyItem[]) => {
    setCustomKeys(keys)
    try {
      localStorage.setItem(STORAGE_KEY_CUSTOM_KEYS, JSON.stringify(keys))
    } catch {
      // ignore
    }
  }

  const handleCheck = () => {
    mutation.mutate(customKeys)
    setCheckSignal((value) => value + 1)
  }

  const handleKiroReport = (report: KiroQuotaReport) => {
    const timestamp = Date.now()
    setKiroReport(report)
    setKiroCachedAt(timestamp)
    try {
      localStorage.setItem('newapi_kiro_quota_cache_v1', JSON.stringify({ report, timestamp }))
    } catch {
      // Ignore unavailable local storage.
    }
  }

  const handleSaveAndCheck = (keys: CustomKeyItem[]) => {
    handleSaveKeys(keys)
    mutation.mutate(keys)
    setCheckSignal((value) => value + 1)
  }

  // Display fresh data if loaded, otherwise fall back to cached data
  const activeReport = mutation.data ?? cachedData?.report ?? null

  return (
    <>
      <SectionPageLayout stackActionsOnMobile>
        <SectionPageLayout.Title>
          <div className='flex flex-col items-start gap-0.5'>
            <span>{t('Quota Check')}</span>
            <span className='text-[13px] font-normal text-muted-foreground'>Cập nhật {Math.max(cachedData?.timestamp ?? 0, kiroCachedAt ?? 0) ? new Date(Math.max(cachedData?.timestamp ?? 0, kiroCachedAt ?? 0)).toLocaleTimeString('vi-VN') : '—'} · {Math.max(cachedData?.timestamp ?? 0, kiroCachedAt ?? 0) ? new Date(Math.max(cachedData?.timestamp ?? 0, kiroCachedAt ?? 0)).toLocaleDateString('vi-VN') : '—'} · dữ liệu đã lưu</span>
            {customKeys.length > 0 && (
              <Badge variant='outline' className='text-xs font-normal'>
                +{customKeys.length} {t('Custom Keys')}
              </Badge>
            )}
          </div>
        </SectionPageLayout.Title>

        <SectionPageLayout.Actions>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              onClick={() => setManageKeysOpen(true)}
              className='gap-1.5'
            >
              <Plus className='size-4' />
              <span>{t('Manage Keys')}</span>
            </Button>

            <Button
              onClick={handleCheck}
              disabled={mutation.isPending}
              className='gap-1.5'
            >
              <RefreshCw
                className={cn('size-4', mutation.isPending && 'animate-spin')}
              />
              <span>{mutation.isPending ? t('Checking...') : 'Kiểm tra tất cả'}</span>
            </Button>
          </div>
        </SectionPageLayout.Actions>

        <SectionPageLayout.Content>
          <div className='space-y-5 [&_[data-slot=card-header]]:px-5 [&_[data-slot=card-content]]:px-5'>
            <div className='grid grid-cols-1 items-stretch gap-6 md:grid-cols-3'>
              <XPikiSourceCard
                report={activeReport}
                onCheck={handleCheck}
                pending={mutation.isPending}
              />
              <KiroQuotaCard report={kiroReport} onReport={handleKiroReport} checkSignal={checkSignal} tokenKeys={tokenCustomKeys} />
              <KiroTokenQuotaCard report={kiroReport} onCheckKey={checkTokenKey} />
            </div>
            <SourceKeyDetails xpiki={activeReport} kiro={kiroReport} />
            {activeReport && (
              <>
                <ModelRatioTable profiles={activeReport.model_profiles} customKeys={customKeys} />
              </>
            )}

            {mutation.isError && !activeReport && (
              <ErrorState title={t('Quota check failed')} onRetry={handleCheck} />
            )}

            {!activeReport && !mutation.isPending && !mutation.isError && (
              <EmptyState
                title={t('Quota has not been checked')}
                description={t('Press Check to query the latest quota.')}
              />
            )}

          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ManageKeysDialog
        open={manageKeysOpen}
        onOpenChange={setManageKeysOpen}
        channelKeys={activeReport?.keys?.filter((k) => !!k.channel_id) ?? []}
        customKeys={customKeys}
        onSaveKeys={handleSaveKeys}
        onSaveAndCheck={handleSaveAndCheck}
      />
    </>
  )
}

export function XPikiRequestLogsPage() {
  const { t } = useTranslation()
  const [customKeys] = useState<CustomKeyItem[]>([])
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Nhật ký yêu cầu')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <XPikiLogsCard customKeys={customKeys} />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
