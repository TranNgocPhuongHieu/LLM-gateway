import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import { useQueryClient } from '@tanstack/react-query'

type Config = { enabled: boolean; environment: string; merchant_id: string; secret_key_configured: boolean; ipn_secret_configured: boolean; ipn_url: string; payment_compliance_confirmed: boolean; bank_code?:string; bank_account?:string; bank_holder?:string; webhook_url?:string; webhook_secret_configured?:boolean }
export function SepaySettings() {
  const { i18n } = useTranslation()
  const vi = normalizeInterfaceLanguage(i18n.resolvedLanguage || i18n.language) === 'vi'
  const s = (vn: string, en: string) => vi ? vn : en
  const [config, setConfig] = useState<Config | null>(null)
  const [secret, setSecret] = useState('')
  const [ipnSecret, setIpnSecret] = useState('')
  const [webhookSecret, setWebhookSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [accepted, setAccepted] = useState(false)
  const [savedEnabled, setSavedEnabled] = useState(false)
  const queryClient = useQueryClient()
  async function load() {
    const { data } = await api.get('/api/option/sepay')
    if (!data.success) throw new Error('load failed')
    setConfig(data.data)
    setSavedEnabled(data.data.enabled)
  }
  useEffect(() => { void load().catch(() => setMessage(s('Không tải được cấu hình SePay.', 'Could not load SePay configuration.'))) }, [])
  async function save() {
    if (!config) return
    if (config.enabled && (!savedEnabled || !config.payment_compliance_confirmed) && !accepted) {
      setMessage(s('Vui lòng xác nhận đã cấu hình IPN và trách nhiệm vận hành trước khi bật nạp.', 'Confirm IPN setup and payment operation responsibilities before enabling top-ups.')); return
    }
    setBusy(true); setMessage('')
    try {
      if (config.enabled && !config.payment_compliance_confirmed && accepted) {
        const confirmation = await api.post('/api/option/payment_compliance', { confirmed: true })
        if (!confirmation.data.success) { setMessage(s('Không xác nhận được. Vui lòng đăng nhập bằng tài khoản chủ hệ thống.', 'Could not confirm. Sign in with the owner account.')); return }
        void queryClient.invalidateQueries({ queryKey: ['system-options'] })
      }
      const { data } = await api.patch('/api/option/sepay', { enabled: config.enabled, environment: config.environment, merchant_id: config.merchant_id, bank_code:config.bank_code || '',bank_account:config.bank_account || '',bank_holder:config.bank_holder || '', ...(webhookSecret ? {webhook_secret:webhookSecret}:{}), ...(secret ? { secret_key: secret } : {}), ...(ipnSecret ? { ipn_secret: ipnSecret } : {}) })
      if (!data.success) { setMessage(data.message || s('Không lưu được.', 'Could not save.')); return }
      setSecret(''); setIpnSecret(''); setWebhookSecret(''); await load()
      setMessage(s('Đã lưu cấu hình SePay.', 'SePay configuration saved.'))
    } catch { setMessage(s('Không lưu được cấu hình. Kiểm tra kết nối và quyền quản trị.', 'Could not save configuration. Check connection and administrator permissions.')) }
    finally { setBusy(false) }
  }
  return <section className='mb-6 rounded-xl border bg-card p-5'>
    <h2 className='font-semibold'>{s('Nạp QR ngân hàng · SePay', 'Bank QR top-up · SePay')}</h2>
    <p className='mt-2 text-sm text-muted-foreground'>{s('Khách chọn số tiền → quét QR → tự động nhận credit. 1 credit = 1.000 VND, nạp tối thiểu 1.000 VND.', 'Choose an amount → scan the QR → receive credits automatically. 1 credit = VND 1,000; minimum top-up VND 1,000.')}</p>
    {config && <div className='mt-4 grid gap-4'>
      <p className='text-sm'>{config.merchant_id && config.secret_key_configured && config.ipn_secret_configured ? s('✓ Đã có thông tin kết nối SePay.', '✓ SePay credentials are configured.') : s('Chưa đủ thông tin kết nối. Mở mục thông tin kết nối bên dưới để nhập.', 'Connection details are incomplete. Open connection details below.')}</p>
      <div className='rounded-lg border bg-muted/30 p-3 text-sm'>
        <p>{s('Trong SePay, bật IPN với URL dưới đây; chọn Secret Key và application/json.', 'In SePay, enable IPN using the URL below; select Secret Key and application/json.')}</p>
        <label className='mt-2 block'>IPN URL<Input className='mt-2' readOnly value={config.ipn_url} onFocus={event => event.target.select()} /></label>
      </div>
      <fieldset className='grid gap-3 rounded-lg border p-3'>
        <legend className='px-1 text-sm font-medium'>{s('Webhook ngân hàng — QR trực tiếp', 'Bank webhook — direct QR')}</legend>
        <p className='text-sm text-muted-foreground'>{s('Trong SePay Webhooks: chọn Tiền vào, JSON, bật tự gửi lại và API Key. Khóa dưới đây phải trùng khóa tại bước Bảo mật; không dùng lại Merchant Secret.', 'In SePay Webhooks: select incoming transfers, JSON, retries and API Key. Enter the same key as in the Security step; do not reuse the Merchant Secret.')}</p>
        <label className='text-sm'>Webhook URL<Input className='mt-1' readOnly value={config.webhook_url || ''} /></label>
        <label className='text-sm'>{s('Mã ngân hàng', 'Bank code')}<Input className='mt-1' value={config.bank_code || ''} onChange={event=>setConfig({...config,bank_code:event.target.value.trim().toUpperCase()})} placeholder='TPBANK' /></label>
        <label className='text-sm'>{s('Số tài khoản', 'Account number')}<Input className='mt-1' value={config.bank_account || ''} onChange={event=>setConfig({...config,bank_account:event.target.value.trim()})} /></label>
        <label className='text-sm'>{s('Chủ tài khoản', 'Account holder')}<Input className='mt-1' value={config.bank_holder || ''} onChange={event=>setConfig({...config,bank_holder:event.target.value})} /></label>
        <label className='text-sm'>Webhook API Key {config.webhook_secret_configured ? '✓' : ''}<Input className='mt-1' type='password' autoComplete='new-password' value={webhookSecret} onChange={event=>setWebhookSecret(event.target.value)} /></label>
      </fieldset>
      <label className='flex items-center gap-3'><Switch checked={config.enabled} onCheckedChange={enabled => setConfig({ ...config, enabled })} />{s('Bật nhận đơn nạp mới', 'Enable new top-ups')}</label>
      {config.enabled && (!savedEnabled || !config.payment_compliance_confirmed) && <label className='flex items-start gap-3 rounded-lg border p-3 text-sm leading-6'>
        <input type='checkbox' checked={accepted} onChange={event => setAccepted(event.target.checked)} className='mt-1' />
        {s('Tôi đã bật IPN đúng cấu hình trên SePay, có quyền cung cấp dịch vụ và chịu trách nhiệm thu tiền, đối soát, bảo vệ dữ liệu khách hàng theo pháp luật áp dụng.', 'I have enabled IPN with the correct SePay settings, am authorized to provide this service, and accept responsibility for payments, reconciliation and customer data protection under applicable law.')}
      </label>}
      <details className='rounded-lg border p-3' open={!config.secret_key_configured}>
      <summary className='cursor-pointer text-sm font-medium'>{s('Thông tin kết nối', 'Connection details')}</summary>
      <div className='mt-4 grid gap-4'>
      <label className='text-sm'>{s('Môi trường', 'Environment')}<select className='mt-2 block rounded-md border bg-background p-2' value={config.environment} onChange={event => setConfig({ ...config, environment: event.target.value })}><option value='production'>Live</option><option value='sandbox'>Sandbox</option></select></label>
      <label className='text-sm'>Merchant ID<Input className='mt-2' value={config.merchant_id} onChange={event => setConfig({ ...config, merchant_id: event.target.value })} autoComplete='off' /></label>
      <label className='text-sm'>Merchant Secret {config.secret_key_configured ? '✓' : ''}<Input className='mt-2' type='password' value={secret} onChange={event => setSecret(event.target.value)} autoComplete='new-password' /></label>
      <label className='text-sm'>IPN Secret {config.ipn_secret_configured ? '✓' : ''}<Input className='mt-2' type='password' value={ipnSecret} onChange={event => setIpnSecret(event.target.value)} autoComplete='new-password' /></label>
      <p className='text-sm text-muted-foreground'>{s('IPN Secret phải khớp giá trị cấu hình Auth Type = Secret Key trên SePay (header X-Secret-Key). Không tự giả định bằng Merchant Secret.', 'IPN Secret must match SePay Auth Type = Secret Key (X-Secret-Key header). Do not assume it equals the Merchant Secret.')}</p>
      </div></details>
      <Button type='button' disabled={busy} onClick={() => void save()} className='w-fit'>{s('Lưu SePay', 'Save SePay')}</Button>
    </div>}
    {message && <p role='status' className='mt-3 text-sm'>{message}</p>}
  </section>
}
