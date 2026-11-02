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

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import type { XPikiTokenCost } from './token-cost'

export type XPikiQuotaKeyResult = {
  label: string
  name?: string
  channel_id?: number
  channel_name?: string
  channel_status?: number
  used_quota?: number
  limit_quota?: number
  remaining_quota?: number
  used_vnd?: string
  limit_vnd?: string
  remaining_vnd?: string
  used_astra: string
  limit_astra: string
  remaining_astra: string
  used_percent: string
  unlimited: boolean
  status: string[]
  error?: string
  reference_model?: string
  used_tokens?: string
  limit_tokens?: string
  remaining_tokens?: string
}

export type XPikiQuotaModelRow = {
  model: string
  model_slug: string
  price_vnd_per_1m: string
  tokens_per_quota: string
  output_price_vnd_per_1m: string
  cache_read_vnd_per_1m: string
  cache_write_vnd_per_1m: string
  tokens_for_26000: string
  effective_input_vnd_per_1m?: string
  effective_output_vnd_per_1m?: string
  effective_cache_read_vnd_per_1m?: string
  observed_multiplier?: string
  verified_samples: number
  verified_at?: string
  unverified_samples: number
  observed_tariffs?: Array<{
    multiplier: string
    input_vnd_per_1m: string
    output_vnd_per_1m: string
    cache_read_vnd_per_1m?: string
    samples: number
    cache_samples: number
    latest_at: string
  }>
}

export type XPikiQuotaModelProfile = {
  labels: string[]
  group_ratio: string
  astra_price_vnd_per_1m: string
  models: XPikiQuotaModelRow[]
  price_basis: string
  cache_price_basis: string
  pricing_warning?: string
  pricing_checked_at?: string
}

export type XPikiQuotaReport = {
  keys: XPikiQuotaKeyResult[]
  total_remaining: string
  total_remaining_vnd?: string
  total_unlimited: boolean
  total_incomplete: boolean
  model_profiles: XPikiQuotaModelProfile[]
  no_model_price_keys: string[]
}

type APIResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export type CustomKeyItem = {
  id?: string
  label?: string
  key: string
}

export async function checkXPikiQuota(
  customKeys?: Array<{ label?: string; key: string }>,
  includePricing = false
): Promise<XPikiQuotaReport> {
  const response = await api.post<APIResponse<XPikiQuotaReport>>(
    '/api/channel/xpiki_quota/check',
    {
      custom_keys: customKeys ?? [],
      include_pricing: includePricing,
    }
  )
  return requireServerSuccess(response.data).data
}

export type KiroQuotaKeyResult = {
  label: string
  quota_mode?: string
  used_tokens?: number
  limit_tokens?: number
  remaining_tokens?: number
  name?: string
  used_credits: number
  limit_credits: number
  remaining_credits: number
  used_percent: number
  unlimited: boolean
  status: string
  error?: string
}

export type KiroQuotaReport = {
  keys: KiroQuotaKeyResult[]
  total_remaining: number
  total_limit: number
  total_used: number
}

export async function checkKiroQuota(
  customKeys?: Array<{ label?: string; key: string }>
): Promise<KiroQuotaReport> {
  const response = await api.post<APIResponse<KiroQuotaReport>>(
    '/api/channel/kiro_quota/check',
    { custom_keys: customKeys ?? [] }
  )
  return requireServerSuccess(response.data).data
}

export async function updateChannelStatus(
  id: number,
  status: number
): Promise<boolean> {
  const response = await api.post<APIResponse<boolean>>(
    `/api/channel/${id}/status`,
    { status }
  )
  return requireServerSuccess(response.data).data
}

export type XPikiLogItem = {
  log_key?: string
  source_key_id?: string
  source_channels?: Array<{ id: number; name: string }>
  request_origin?: 'newapi' | 'usage_match' | 'unmatched' | 'ambiguous'
  origin_match?: 'request_id' | 'usage_time'
  local_request_id?: string
  local_final_status?: 'success'
  fallback_channel_id?: number
  fallback_channel_name?: string
  log_id?: number
  created_at: string
  type: string
  model_slug: string
  status: string
  error_code?: string
  request_outcome?:
    | 'success'
    | 'retry_recovered'
    | 'failed_final'
    | 'cancelled'
    | 'stream_failed'
    | 'stream_incomplete'
    | 'unknown'
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  credit_cost: number
  quota_astra?: string
  cost_reference_model?: string
  charged_reference_tokens?: string
  quota_source?: 'xpiki' | 'unmatched' | 'unavailable' | 'not_xpiki'
  upstream_quota?: string
  upstream_request_id?: string
  cost_vnd: string
  token_cost?: XPikiTokenCost
  duration_ms: number
  request_id: string
  ip: string
  key_label: string
  key_masked?: string
  token_name?: string
  username?: string
  token_id?: number
  request_path?: string
  channel_id?: number
  channel_name?: string
  destination?: string
  retried?: boolean
  initial_channel_id?: number
  initial_channel_name?: string
  initial_destination?: string
  initial_error_code?: string
  channel_trail?: string[]
  total_attempts?: number
  failed_attempts?: number
  attempt_number?: number
  attempts?: XPikiLogItem[]
}

export type XPikiKeyOption = {
  value: string
  label: string
  channel_id?: number
  channel_name?: string
  key_masked: string
}

export type XPikiLogsReport = {
  source?: 'xpiki' | 'newapi'
  incomplete?: boolean
  items: XPikiLogItem[]
  total: number
  key_options: XPikiKeyOption[]
  model_options?: string[]
  user_options?: string[]
  has_more: boolean
  quota_errors?: string[]
  token_cost_summary?: {
    known_requests: number
    unknown_requests: number
    total_vnd: string
    cost_vnd: string
  }
}

export type FetchXPikiLogsParams = {
  source?: 'xpiki' | 'newapi'
  custom_keys?: Array<{ label?: string; key: string }>
  key_filter?: string
  channel_filter?: string
  search?: string
  model?: string
  user?: string
  status?: string
  type?: string
  date?: string
  start_date?: string
  end_date?: string
  page?: number
  page_size?: number
  group_mode?: 'grouped' | 'all'
}

export async function fetchXPikiLogs(
  params: FetchXPikiLogsParams,
  signal?: AbortSignal
): Promise<XPikiLogsReport> {
  const response = await api.post<APIResponse<XPikiLogsReport>>(
    '/api/channel/xpiki_quota/logs',
    params,
    { signal }
  )
  return requireServerSuccess(response.data).data
}
