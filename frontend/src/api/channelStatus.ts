/**
 * 渠道状态公开 API（GET /api/v1/channel-status，可匿名访问）
 * 数据来自后台对真实生成记录（用量日志 + 错误日志）的定时聚合快照，非实时查询。
 * 聚合维度：分组 × 模型。管理后台开启「渠道状态统计」后可用；关闭时后端返回 404。
 * 契约文档见 deploy/CHANNEL_STATUS_API.md。
 */

import { apiClient } from './client'

export type ChannelHealthStatus = 'operational' | 'degraded' | 'down' | 'unknown'

/** 时间线单桶（桶宽 = 统计窗口 / 24）。success_rate 为 -1 表示该桶无请求。 */
export interface ChannelStatusBucket {
  bucket_start_ms: number
  requests: number
  failures: number
  success_rate: number
  status: ChannelHealthStatus
}

/** 按模型细分的同口径统计（按请求量降序）。 */
export interface ChannelStatusModelStat {
  model: string
  requests: number
  failures: number
  success_rate: number
  avg_gen_ms: number
}

/** 单个分组的可用性状态。 */
export interface ChannelStatusGroup {
  id: number
  name: string
  platform: string
  status: ChannelHealthStatus
  requests: number
  failures: number
  success_rate: number
  avg_gen_ms: number
  models: ChannelStatusModelStat[]
  timeline: ChannelStatusBucket[]
}

/** 聚合快照（缓存读取）。首轮聚合未完成时 groups 为空。 */
export interface ChannelStatusSnapshot {
  generated_at_ms: number
  window_hours: number
  refresh_seconds: number
  /** 全局状态：所有分组的最差值；无数据为 unknown。 */
  overall: ChannelHealthStatus
  groups: ChannelStatusGroup[]
}

/** 获取渠道状态快照；功能未启用（后端 404）时抛错，由调用方区分处理。 */
export async function getChannelStatus(options?: {
  signal?: AbortSignal
}): Promise<ChannelStatusSnapshot> {
  const { data } = await apiClient.get<ChannelStatusSnapshot>('/channel-status', {
    signal: options?.signal
  })
  return data
}

export const channelStatusAPI = { getChannelStatus }

export default channelStatusAPI
