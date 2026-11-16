import { apiClient } from '../client'

// 利润统计（仅管理员）。
// 口径：收入 = 消费(actual_cost) × 折扣率（用户覆盖或全局默认）；
// 成本 = account_stats_cost（渠道"账号统计定价"快照，未配置时按售价×账号倍率估算）。

export interface ProfitTotals {
  requests: number
  billed_requests: number
  configured_cost_requests: number
  image_count: number
  gross_revenue: number
  revenue: number
  cost: number
}

export interface ProfitDailyPoint {
  day: string
  requests: number
  revenue: number
  cost: number
  profit: number
}

export interface ProfitSummaryResponse {
  start: string
  end: string
  totals: ProfitTotals
  daily: ProfitDailyPoint[]
}

export interface ProfitUserRow {
  user_id: number
  email: string
  name: string
  rate: number
  rate_set: boolean
  requests: number
  images: number
  revenue: number
  cost: number
  profit: number
  margin: number
}

export interface ProfitUsersResponse {
  items: ProfitUserRow[]
  total: number
  page: number
  page_size: number
}

export interface ProfitModelRow {
  model: string
  platform: string
  requests: number
  images: number
  gross_revenue: number
  revenue: number
  cost: number
  profit: number
  margin: number
}

export interface ProfitAccountRow {
  account_id: number
  account_name: string
  platform: string
  requests: number
  images: number
  revenue: number
  cost: number
  profit: number
  margin: number
}

export interface UserProfitDetailResponse {
  user_id: number
  rate: number
  rate_set: boolean
  totals: ProfitTotals
  by_model: ProfitModelRow[]
  by_account: ProfitAccountRow[]
  daily: ProfitDailyPoint[]
}

export interface ProfitUserRate {
  user_id: number
  email: string
  name: string
  rate: number
  note: string
  updated_at: string
}

export interface ProfitConfig {
  default_user_rate: number
}

export interface ProfitRangeParams {
  start?: string
  end?: string
}

export async function getProfitSummary(params: ProfitRangeParams): Promise<ProfitSummaryResponse> {
  const { data } = await apiClient.get('/admin/profit/summary', { params })
  return data
}

export async function getProfitUsers(
  params: ProfitRangeParams & { search?: string; order?: string; page?: number; page_size?: number },
): Promise<ProfitUsersResponse> {
  const { data } = await apiClient.get('/admin/profit/users', { params })
  return data
}

export async function getProfitUserDetail(
  userId: number,
  params: ProfitRangeParams,
): Promise<UserProfitDetailResponse> {
  const { data } = await apiClient.get(`/admin/profit/users/${userId}`, { params })
  return data
}

export async function getProfitModels(params: ProfitRangeParams): Promise<{ items: ProfitModelRow[] }> {
  const { data } = await apiClient.get('/admin/profit/models', { params })
  return data
}

export async function getProfitAccounts(params: ProfitRangeParams): Promise<{ items: ProfitAccountRow[] }> {
  const { data } = await apiClient.get('/admin/profit/accounts', { params })
  return data
}

export async function getProfitRates(): Promise<{ items: ProfitUserRate[] }> {
  const { data } = await apiClient.get('/admin/profit/rates')
  return data
}

export async function updateProfitRate(userId: number, rate: number, note: string): Promise<void> {
  await apiClient.put(`/admin/profit/rates/${userId}`, { rate, note })
}

export async function deleteProfitRate(userId: number): Promise<void> {
  await apiClient.delete(`/admin/profit/rates/${userId}`)
}

export async function getProfitConfig(): Promise<ProfitConfig> {
  const { data } = await apiClient.get('/admin/profit/config')
  return data
}

export async function updateProfitConfig(defaultUserRate: number): Promise<void> {
  await apiClient.put('/admin/profit/config', { default_user_rate: defaultUserRate })
}

// ─── 成本规则（利润模块私有）───

export interface ProfitCostRule {
  id: number
  account_id: number | null
  account_name: string
  model: string
  billing_mode: 'token' | 'per_image' | 'per_request'
  input_price: number
  output_price: number
  image_price: number
  request_price: number
  enabled: boolean
  note: string
  updated_at: string
}

export interface CostRulePayload {
  account_id: number | null
  model: string
  billing_mode: 'token' | 'per_image' | 'per_request'
  input_price: number
  output_price: number
  image_price: number
  request_price: number
  enabled: boolean
  note: string
}

export async function getCostRules(): Promise<{ items: ProfitCostRule[] }> {
  const { data } = await apiClient.get('/admin/profit/cost-rules')
  return data
}

export async function createCostRule(payload: CostRulePayload): Promise<void> {
  await apiClient.post('/admin/profit/cost-rules', payload)
}

export async function updateCostRule(id: number, payload: CostRulePayload): Promise<void> {
  await apiClient.put(`/admin/profit/cost-rules/${id}`, payload)
}

export async function deleteCostRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/profit/cost-rules/${id}`)
}

// 成本规则可选模型（账号映射键/值 + 近30天实际调用模型）
export async function getCostRuleModels(accountId: number | null): Promise<{ items: string[] }> {
  const { data } = await apiClient.get('/admin/profit/cost-rules/models', {
    params: accountId ? { account_id: accountId } : {},
  })
  return data
}

// 批量保存某账号范围的全部成本规则（未配置价格的模型不生成规则 = 成本0）
export interface BatchCostRuleRow {
  model: string
  billing_mode: 'token' | 'per_image' | 'per_request'
  input_price: number
  output_price: number
  image_price: number
  request_price: number
  note: string
}

export async function replaceCostRules(accountId: number | null, rules: BatchCostRuleRow[]): Promise<void> {
  await apiClient.post('/admin/profit/cost-rules/batch', { account_id: accountId, rules })
}
