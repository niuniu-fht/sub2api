import { apiClient } from '../client'

// 上游余额监控（对齐 new-api 渠道余额）：
//   - OpenAI 兼容中转站：/v1/dashboard/billing/subscription + usage → hard_limit_usd - total_usage/100
//   - Sub2API 站：/v1/usage → remaining/balance

export interface UpstreamBalanceSnapshot {
  status: 'success' | 'error' | 'unsupported'
  protocol?: string
  balance?: number
  hard_limit?: number
  used?: number
  currency?: string
  checked_at?: string
  error?: string
  http_status?: number
}

export interface UpstreamBalanceListItem {
  account_id: number
  account_name: string
  platform: string
  type: string
  schedulable: boolean
  group_ids?: number[]
  base_url_host?: string
  queryable: boolean
  snapshot?: UpstreamBalanceSnapshot | null
}

export interface UpstreamBalanceResult {
  account_id: number
  status: 'success' | 'error' | 'unsupported'
  protocol?: string
  balance?: number
  hard_limit?: number
  used?: number
  currency?: string
  error?: string
  persisted?: boolean
}

export async function getUpstreamBalances(): Promise<{ items: UpstreamBalanceListItem[] }> {
  const { data } = await apiClient.get('/admin/upstream-balances')
  return data
}

export async function refreshUpstreamBalance(accountId: number): Promise<UpstreamBalanceResult> {
  const { data } = await apiClient.post(`/admin/accounts/${accountId}/upstream-balance`)
  return data
}

export async function refreshUpstreamBalances(accountIds?: number[]): Promise<{ items: UpstreamBalanceResult[] }> {
  const { data } = await apiClient.post('/admin/upstream-balances/refresh', {
    account_ids: accountIds,
  })
  return data
}
