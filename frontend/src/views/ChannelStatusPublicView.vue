<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <PlazaNavBar login-redirect="/status" />

    <main class="mx-auto max-w-5xl px-4 py-8 sm:px-6">
      <!-- 标题 -->
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('statusPage.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('statusPage.subtitle') }}
        </p>
      </div>

      <!-- 加载中 -->
      <div v-if="loading" class="flex justify-center py-20">
        <LoadingSpinner />
      </div>

      <!-- 功能未启用（后端 404） -->
      <EmptyState
        v-else-if="disabled"
        :title="t('statusPage.disabledTitle')"
        :description="t('statusPage.disabledDesc')"
      />

      <!-- 加载失败 -->
      <div v-else-if="loadFailed" class="py-16 text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('statusPage.loadFailed') }}</p>
        <button
          type="button"
          class="mt-4 inline-flex items-center rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700"
          @click="load"
        >
          {{ t('statusPage.retry') }}
        </button>
      </div>

      <template v-else-if="snapshot">
        <!-- 整体状态横幅 -->
        <section
          class="mb-6 flex flex-col gap-4 rounded-2xl border p-5 sm:flex-row sm:items-center sm:justify-between"
          :class="overallCardClass"
          data-testid="status-overall"
        >
          <div class="flex items-center gap-3">
            <span class="h-3 w-3 flex-shrink-0 rounded-full" :class="STATUS_DOT[snapshot.overall]"></span>
            <div>
              <div class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ overallLabel }}
              </div>
              <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('statusPage.updatedAt', { time: updatedLabel }) }}
                · {{ t('statusPage.windowHours', { n: snapshot.window_hours }) }}
                · {{ t('statusPage.refreshEvery', { n: snapshot.refresh_seconds }) }}
              </div>
            </div>
          </div>
          <AutoRefreshButton
            :enabled="autoRefresh.enabled.value"
            :interval-seconds="autoRefresh.intervalSeconds.value"
            :countdown="autoRefresh.countdown.value"
            :intervals="autoRefresh.intervals"
            @update:enabled="autoRefresh.setEnabled"
            @update:interval="autoRefresh.setInterval"
          />
        </section>

        <!-- 首轮聚合未完成 -->
        <EmptyState
          v-if="snapshot.channels.length === 0"
          :title="t('statusPage.preparing')"
        />

        <template v-else>
          <!-- 图例 -->
          <div class="mb-3 flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
            <span class="font-medium">{{ t('statusPage.legend') }}</span>
            <span v-for="s in LEGEND_STATUSES" :key="s" class="inline-flex items-center gap-1.5">
              <span class="h-2 w-2 rounded-full" :class="STATUS_DOT[s]"></span>
              {{ statusLabel(s) }}
            </span>
          </div>

          <!-- 渠道列表 -->
          <div class="space-y-4" data-testid="status-channel-list">
            <article
              v-for="channel in sortedChannels"
              :key="channel.id"
              class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800"
            >
              <!-- 头部：名称/平台 + 状态 + 成功率 -->
              <div class="flex items-start justify-between gap-3">
                <div class="flex min-w-0 items-center gap-2.5">
                  <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-gray-100 ring-1 ring-black/5 dark:bg-dark-700 dark:ring-white/10">
                    <PlatformIcon :platform="channel.platform as GroupPlatform" size="sm" />
                  </span>
                  <div class="min-w-0">
                    <div class="truncate text-base font-semibold text-gray-900 dark:text-gray-100">
                      {{ channel.name }}
                    </div>
                    <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t('statusPage.requests') }} {{ channel.requests }}
                      · {{ t('statusPage.failures') }} {{ channel.failures }}
                    </div>
                  </div>
                </div>
                <div class="flex flex-shrink-0 flex-col items-end gap-1">
                  <span
                    class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold"
                    :class="STATUS_CHIP[channel.status]"
                  >
                    <span class="h-1.5 w-1.5 rounded-full" :class="STATUS_DOT[channel.status]"></span>
                    {{ statusLabel(channel.status) }}
                  </span>
                  <span class="text-sm font-semibold tabular-nums text-gray-700 dark:text-gray-200">
                    {{ t('statusPage.successRate') }}
                    {{ formatRate(channel.success_rate) }}
                  </span>
                </div>
              </div>

              <!-- 关键指标 -->
              <div class="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                <span>{{ t('statusPage.avgGenTime') }}: <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(channel.avg_gen_ms) }}</span></span>
              </div>

              <!-- 时间线（最旧在左，最新在右） -->
              <div class="mt-4 pt-3 border-t border-gray-100 dark:border-dark-700/60">
                <div class="mb-2 flex justify-between text-[10px] font-semibold uppercase tracking-widest text-gray-400">
                  <span>{{ t('statusPage.timeline', { n: snapshot.window_hours }) }}</span>
                  <span class="tabular-nums">{{ t('statusPage.updatedAt', { time: updatedLabel }) }}</span>
                </div>
                <div class="flex h-5 w-full items-stretch gap-[2px]">
                  <div
                    v-for="(bucket, idx) in orderedTimeline(channel)"
                    :key="idx"
                    class="min-w-0 flex-1 rounded-sm"
                    :class="STATUS_BAR[bucket.status] ?? STATUS_BAR.unknown"
                    :title="bucketTitle(bucket)"
                  ></div>
                </div>
                <div class="mt-1 flex justify-between text-[9px] uppercase tracking-widest text-gray-400">
                  <span>{{ t('statusPage.past') }}</span>
                  <span>{{ t('statusPage.now') }}</span>
                </div>
              </div>

              <!-- 模型明细（可展开） -->
              <div v-if="channel.models.length > 0" class="mt-3">
                <button
                  type="button"
                  class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  :data-testid="`status-models-toggle-${channel.id}`"
                  @click="toggleModels(channel.id)"
                >
                  {{ expanded[channel.id]
                    ? t('common.collapse')
                    : t('statusPage.models') }}
                </button>
                <div v-if="expanded[channel.id]" class="mt-2 overflow-x-auto">
                  <table class="w-full text-left text-xs">
                    <thead>
                      <tr class="text-[10px] uppercase tracking-widest text-gray-400">
                        <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.modelCol') }}</th>
                        <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.requestsCol') }}</th>
                        <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.failuresCol') }}</th>
                        <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.successRateCol') }}</th>
                        <th class="py-1.5 font-semibold">{{ t('statusPage.avgGenCol') }}</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                      <tr v-for="m in channel.models" :key="m.model">
                        <td class="py-1.5 pr-4 font-mono text-gray-800 dark:text-gray-200">{{ m.model }}</td>
                        <td class="py-1.5 pr-4 tabular-nums text-gray-600 dark:text-gray-300">{{ m.requests }}</td>
                        <td class="py-1.5 pr-4 tabular-nums text-gray-600 dark:text-gray-300">{{ m.failures }}</td>
                        <td class="py-1.5 pr-4 tabular-nums text-gray-600 dark:text-gray-300">{{ formatRate(m.success_rate) }}</td>
                        <td class="py-1.5 tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(m.avg_gen_ms) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </article>
          </div>
        </template>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { getChannelStatus, type ChannelHealthStatus, type ChannelStatusBucket, type ChannelStatusChannel, type ChannelStatusSnapshot } from '@/api/channelStatus'
import { useAutoRefresh } from '@/composables/useAutoRefresh'
import { useAppStore } from '@/stores/app'
import { formatDateTimeToMinute } from '@/utils/format'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const snapshot = ref<ChannelStatusSnapshot | null>(null)
const loading = ref(true)
const disabled = ref(false)
const loadFailed = ref(false)
const expanded = ref<Record<number, boolean>>({})

const LEGEND_STATUSES: ChannelHealthStatus[] = ['operational', 'degraded', 'down', 'unknown']

// 状态 → 展示样式（点/胶囊/条块）。静态配色，无动画。
const STATUS_DOT: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  down: 'bg-red-500',
  unknown: 'bg-gray-300 dark:bg-dark-600'
}

const STATUS_CHIP: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400',
  degraded: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-400',
  down: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400',
  unknown: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

const STATUS_BAR: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  down: 'bg-red-500',
  unknown: 'bg-gray-200 dark:bg-dark-700'
}

// 故障优先展示：down > degraded > operational > unknown，同档按成功率升序。
const STATUS_SEVERITY: Record<ChannelHealthStatus, number> = {
  down: 0,
  degraded: 1,
  operational: 2,
  unknown: 3
}

const sortedChannels = computed<ChannelStatusChannel[]>(() => {
  const list = [...(snapshot.value?.channels ?? [])]
  list.sort((a, b) => {
    const sev = STATUS_SEVERITY[a.status] - STATUS_SEVERITY[b.status]
    if (sev !== 0) return sev
    return a.success_rate - b.success_rate
  })
  return list
})

const overallCardClass = computed(() => {
  const overall = snapshot.value?.overall
  if (overall === 'operational') return 'border-emerald-200 bg-emerald-50/60 dark:border-emerald-500/20 dark:bg-emerald-500/5'
  if (overall === 'degraded') return 'border-amber-200 bg-amber-50/60 dark:border-amber-500/20 dark:bg-amber-500/5'
  if (overall === 'down') return 'border-red-200 bg-red-50/60 dark:border-red-500/20 dark:bg-red-500/5'
  return 'border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800'
})

const overallLabel = computed(() => {
  const key = `statusPage.overall${cap(snapshot.value?.overall ?? 'unknown')}`
  return t(key)
})

const updatedLabel = computed(() => {
  const ms = snapshot.value?.generated_at_ms
  if (!ms) return '—'
  return formatDateTimeToMinute(new Date(ms))
})

function statusLabel(status: ChannelHealthStatus): string {
  return t(`statusPage.status${cap(status)}`)
}

function cap(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

/** success_rate 为 -1 表示窗口内无请求。 */
function formatRate(rate: number): string {
  if (rate < 0) return '—'
  return `${Math.round(rate * 100) / 100}%`
}

function formatGenMs(ms: number): string {
  if (!ms || ms <= 0) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

/** API 返回的桶为最旧在前，直接左→右渲染即为「过去 → 现在」。 */
function orderedTimeline(channel: ChannelStatusChannel): ChannelStatusBucket[] {
  return channel.timeline ?? []
}

function bucketTitle(bucket: ChannelStatusBucket): string {
  const time = formatDateTimeToMinute(new Date(bucket.bucket_start_ms))
  if (bucket.status === 'unknown') return `${time} · ${statusLabel('unknown')}`
  const rate = bucket.success_rate < 0 ? '—' : `${bucket.success_rate.toFixed(1)}%`
  return `${time} · ${statusLabel(bucket.status)} · ${bucket.requests}✓/${bucket.failures}✗ · ${rate}`
}

function toggleModels(id: number): void {
  expanded.value = { ...expanded.value, [id]: !expanded.value[id] }
}

async function load(): Promise<void> {
  try {
    snapshot.value = await getChannelStatus()
    disabled.value = false
    loadFailed.value = false
  } catch (err) {
    const status = (err as { status?: number })?.status
    if (status === 404) {
      disabled.value = true
      snapshot.value = null
    } else {
      loadFailed.value = true
    }
  } finally {
    loading.value = false
  }
}

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-page-autorefresh',
  intervals: [30, 60, 120, 300],
  defaultInterval: 60,
  defaultEnabled: true,
  onRefresh: load,
  shouldPause: () => document.hidden
})

onMounted(() => {
  // 独立形态导航条需要站点名/Logo。
  void appStore.fetchPublicSettings()
  void load()
  // defaultEnabled/本地记忆为开启时需显式启动定时器（组合式函数不会自启）。
  autoRefresh.setEnabled(autoRefresh.enabled.value)
})
</script>
