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
import type { ApiRequestConfig } from '@/lib/http-client'

import type {
  PaymentRequest,
  AmountRequest,
  ApiResponse,
  TopupInfoResponse,
  AmountResponse,
  PaymentResponse,
  StripePaymentResponse,
  BillingHistoryResponse,
  CompleteOrderRequest,
} from './types'

// ============================================================================
// Wallet API Functions
// ============================================================================

/**
 * Check if API response is successful
 */
export function isApiSuccess(response: ApiResponse): boolean {
  return response.success === true || response.message === 'success'
}

/**
 * Get topup configuration info
 */
export async function getTopupInfo(): Promise<TopupInfoResponse> {
  const res = await api.get('/api/user/topup/info')
  return res.data
}

/**
 * Calculate payment amount for regular payment
 */
export async function calculateAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Stripe payment
 */
export async function calculateStripeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/stripe/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request regular payment
 */
export async function requestPayment(
  request: PaymentRequest
): Promise<PaymentResponse> {
  const res = await api.post('/api/user/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return {
    ...res.data,
    url: res.data.url || (res as unknown as { url?: string }).url,
  }
}

/**
 * Request Stripe payment
 */
export async function requestStripePayment(
  request: PaymentRequest
): Promise<StripePaymentResponse> {
  const res = await api.post('/api/user/stripe/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get billing history for current user
 */
export async function getUserBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup/self?${params.toString()}`)
  return res.data
}

/**
 * Get billing history for all users (admin only)
 */
export async function getAllBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup?${params.toString()}`)
  return res.data
}

/** Admin summary returned alongside each admin top-up list page (all-time unless noted). */
export interface AdminTopupSummary {
  total: number
  pending: number
  success: number
  cancelled: number
  processing: number
  needs_review: number
  /** Sum of successful top-ups in VND (SePay amount, otherwise money). */
  collected_vnd: number
  /** Same sum restricted to today_start..today_end. */
  today_vnd: number
}

export interface AdminTopupListParams {
  page: number
  pageSize: number
  keyword?: string
  /** 'success' | 'expired' | 'needs_review'; omit for all. */
  status?: string
  /** 'sepay' | 'binance'; omit for all. */
  paymentMethod?: string
  startTimestamp?: number
  endTimestamp?: number
  todayStart?: number
  todayEnd?: number
}

export interface AdminTopupListResponse extends BillingHistoryResponse {
  summary?: AdminTopupSummary
}

/**
 * Admin top-up list with server-side keyword, status, payment method and time
 * filters. Pass `todayStart`/`todayEnd` (local day, unix seconds, end exclusive)
 * to get local-day totals in the summary.
 */
export async function getAdminTopupList(
  params: AdminTopupListParams,
  config?: ApiRequestConfig
): Promise<ApiResponse<AdminTopupListResponse>> {
  const query = new URLSearchParams({
    p: params.page.toString(),
    page_size: params.pageSize.toString(),
  })
  if (params.keyword) query.append('keyword', params.keyword)
  if (params.status) query.append('status', params.status)
  if (params.paymentMethod) query.append('payment_method', params.paymentMethod)
  if (params.startTimestamp) query.append('start_timestamp', params.startTimestamp.toString())
  if (params.endTimestamp) query.append('end_timestamp', params.endTimestamp.toString())
  if (params.todayStart && params.todayEnd) {
    query.append('today_start', params.todayStart.toString())
    query.append('today_end', params.todayEnd.toString())
  }
  const res = await api.get(`/api/user/topup?${query.toString()}`, config)
  return res.data
}

/**
 * Complete a pending order (admin only)
 */
export async function completeOrder(
  request: CompleteOrderRequest
): Promise<ApiResponse> {
  const res = await api.post('/api/user/topup/complete', request)
  return res.data
}

export async function reconcileSepayOrder(tradeNo: string): Promise<ApiResponse> {
  const response = await api.post(`/api/user/sepay/orders/${encodeURIComponent(tradeNo)}/reconcile`)
  return response.data
}

export async function cancelSepayOrder(tradeNo: string): Promise<ApiResponse> {
  const response = await api.post(`/api/user/sepay/orders/${encodeURIComponent(tradeNo)}/cancel`)
  return response.data
}
