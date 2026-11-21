<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6">
      <!-- 页头 -->
      <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamBalance.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.upstreamBalance.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="refreshingAll || loading"
              @click="refreshAll"
            >
              {{ refreshingAll ? t('admin.upstreamBalance.refreshing') : t('admin.upstreamBalance.refreshAll') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 汇总卡片 -->
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstreamBalance.cards.queryable') }}</div>
          <div class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ stats.queryable }}</div>
        </div>
        <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstreamBalance.cards.ok') }}</div>
          <div class="mt-1 text-2xl font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{{ stats.ok }}</div>
        </div>
        <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstreamBalance.cards.failed') }}</div>
          <div class="mt-1 text-2xl font-semibold tabular-nums text-red-600 dark:text-red-400">{{ stats.failed }}</div>
        </div>
        <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstreamBalance.cards.total') }}</div>
          <div class="mt-1 text-2xl font-semibold tabular-nums text-primary-600 dark:text-primary-300">{{ fmtMoney(stats.total) }}</div>
        </div>
      </div>

      <!-- 列表 -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <!-- 过滤器 -->
        <div class="mb-4 flex flex-wrap items-center gap-2">
          <div class="flex flex-wrap gap-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-900">
            <button
              v-for="opt in platformOptions"
              :key="opt.value"
              type="button"
              class="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition"
              :class="filterPlatform === opt.value
                ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="filterPlatform = opt.value"
            >
              <PlatformIcon v-if="opt.value" :platform="opt.value" size="xs" />
              <span>{{ opt.label }}</span>
            </button>
          </div>
          <Select v-model="filterStatus" :options="statusOptions" class="w-40" />
          <label class="flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
            <input v-model="groupByUpstream" type="checkbox" class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 dark:border-dark-500" />
            {{ t('admin.upstreamBalance.groupByUpstream') }}
          </label>
          <label v-if="hiddenCount > 0" class="flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
            <input v-model="showHidden" type="checkbox" class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 dark:border-dark-500" />
            {{ t('admin.upstreamBalance.showHidden', { n: hiddenCount }) }}
          </label>
          <SearchInput
            v-model="filterSearch"
            :placeholder="t('admin.upstreamBalance.searchAccount')"
            class="ml-auto w-52"
          />
        </div>

        <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
                <th class="px-3 py-2.5">{{ t('admin.upstreamBalance.col.account') }}</th>
                <th class="px-3 py-2.5">{{ t('admin.upstreamBalance.col.upstream') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('admin.upstreamBalance.col.balance') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('admin.upstreamBalance.col.used') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('admin.upstreamBalance.col.limit') }}</th>
                <th class="px-3 py-2.5">{{ t('admin.upstreamBalance.col.status') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
              <tr
                v-for="item in rowItems"
                v-show="rowVisible(item)"
                :key="item.account_id"
                class="transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
                :class="isHidden(item.account_id) ? 'opacity-50' : ''"
                :data-testid="`upstream-balance-${item.account_id}`"
              >
                <td class="px-3 py-2.5">
                  <div class="flex items-center gap-2">
                    <span :class="['inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1', platformBadgeClass(item.platform)]">
                      <PlatformIcon :platform="item.platform" size="sm" />
                    </span>
                    <span class="min-w-0 max-w-[220px] truncate font-medium text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</span>
                    <span class="shrink-0 text-xs text-gray-400">#{{ item.account_id }}</span>
                    <span v-if="!item.schedulable" class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-400 dark:bg-dark-700 dark:text-gray-500">{{ t('admin.upstreamBalance.disabled') }}</span>
                    <button
                      v-if="isGroupHead(item)"
                      type="button"
                      class="shrink-0 rounded bg-primary-50 px-1.5 py-0.5 text-[10px] font-medium text-primary-700 transition hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-200 dark:hover:bg-primary-900/50"
                      :title="t('admin.upstreamBalance.groupToggleHint')"
                      @click="toggleHostGroup(item.base_url_host)"
                    >{{ hostExpanded(item.base_url_host) ? '−' : '+' }}{{ hostGroupSize(item.base_url_host) - 1 }}</button>
                  </div>
                </td>
                <td class="px-3 py-2.5 font-mono text-xs text-gray-500 dark:text-gray-400">{{ item.base_url_host || '—' }}</td>
                <td class="px-3 py-2.5 text-right">
                  <span v-if="item.snapshot?.status === 'success' && item.snapshot.unlimited" class="inline-flex items-center gap-1 rounded-md bg-primary-50 px-2 py-1 text-sm font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-200" :title="t('admin.upstreamBalance.unlimitedHint')">
                    ∞ {{ t('admin.upstreamBalance.unlimited') }}
                    <span class="text-xs font-normal text-gray-500 dark:text-gray-400">{{ t('admin.upstreamBalance.usedShort') }} {{ fmtMoney(item.snapshot.used) }}</span>
                  </span>
                  <span v-else-if="item.snapshot?.status === 'success'" class="inline-flex items-center rounded-md px-2 py-1 text-sm font-semibold tabular-nums" :class="balanceClass(item.snapshot.balance)">
                    {{ fmtMoney(item.snapshot.balance) }}
                  </span>
                  <span v-else-if="!item.queryable" class="text-sm text-gray-400">—</span>
                  <span v-else class="text-sm text-gray-400" :title="item.snapshot?.error">?</span>
                </td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ item.snapshot?.used ? fmtMoney(item.snapshot.used) : '—' }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ item.snapshot?.unlimited ? '∞' : (item.snapshot?.hard_limit ? fmtMoney(item.snapshot.hard_limit) : '—') }}</td>
                <td class="px-3 py-2.5">
                  <span v-if="refreshingIds.has(item.account_id)" class="text-xs text-primary-600 dark:text-primary-400">{{ t('admin.upstreamBalance.refreshing') }}…</span>
                  <span v-else-if="item.snapshot?.status === 'success'" class="inline-flex items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400">
                    <span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
                    {{ formatTime(item.snapshot.checked_at) }}
                    <span v-if="item.snapshot.protocol" class="text-gray-400">({{ item.snapshot.protocol }})</span>
                  </span>
                  <span v-else-if="item.snapshot?.status === 'error'" class="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400" :title="item.snapshot.error">
                    <span class="h-1.5 w-1.5 rounded-full bg-red-500"></span>
                    {{ truncateText(item.snapshot.error, 40) }}
                  </span>
                  <span v-else-if="item.snapshot?.status === 'unsupported'" class="text-xs text-gray-400">{{ t('admin.upstreamBalance.unsupported') }}</span>
                  <span v-else class="text-xs text-gray-400">{{ t('admin.upstreamBalance.neverChecked') }}</span>
                </td>
                <td class="px-3 py-2.5 text-right">
                  <div class="flex items-center justify-end gap-2">
                    <button
                      v-if="item.queryable"
                      type="button"
                      class="text-xs font-medium text-primary-600 underline decoration-dashed underline-offset-2 transition-colors hover:text-primary-700 dark:text-primary-400"
                      :disabled="refreshingIds.has(item.account_id)"
                      @click="refreshOne(item)"
                    >
                      {{ t('admin.upstreamBalance.refresh') }}
                    </button>
                    <button
                      type="button"
                      class="text-xs text-gray-400 underline decoration-dashed underline-offset-2 transition-colors hover:text-gray-600 dark:hover:text-gray-200"
                      :title="isHidden(item.account_id) ? t('admin.upstreamBalance.unhideHint') : t('admin.upstreamBalance.hideHint')"
                      @click="toggleHidden(item.account_id)"
                    >
                      {{ isHidden(item.account_id) ? t('admin.upstreamBalance.unhide') : t('admin.upstreamBalance.hide') }}
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!visibleItems.length && !loading">
                <td colspan="7" class="px-3 py-10 text-center text-gray-400">{{ t('admin.upstreamBalance.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import { useAppStore } from '@/stores/app'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import {
  getUpstreamBalances,
  refreshUpstreamBalance,
  refreshUpstreamBalances,
  type UpstreamBalanceListItem,
} from '@/api/admin/upstreamBalance'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const items = ref<UpstreamBalanceListItem[]>([])
const refreshingAll = ref(false)
const refreshingIds = ref<Set<number>>(new Set())

const filterPlatform = ref('')
const filterStatus = ref('')
const filterSearch = ref('')

// ─── 个人偏好（localStorage）：隐藏账号 + 按上游聚合 ───
const HIDDEN_KEY = 'upstream-balance-hidden'
const GROUP_KEY = 'upstream-balance-group'
const hiddenIds = ref<Set<number>>(new Set(JSON.parse(localStorage.getItem(HIDDEN_KEY) || '[]')))
const showHidden = ref(false)
const groupByUpstream = ref(localStorage.getItem(GROUP_KEY) === '1')
const expandedHosts = ref<Set<string>>(new Set())

watch(groupByUpstream, (v) => localStorage.setItem(GROUP_KEY, v ? '1' : '0'))

function isHidden(id: number): boolean {
  return hiddenIds.value.has(id)
}

function toggleHidden(id: number): void {
  const next = new Set(hiddenIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  hiddenIds.value = next
  localStorage.setItem(HIDDEN_KEY, JSON.stringify([...next]))
}

const hiddenCount = computed(() => items.value.filter((i) => hiddenIds.value.has(i.account_id)).length)

const platformOptions = computed(() => [
  { value: '', label: t('admin.upstreamBalance.allPlatforms') },
  ...[...new Set(items.value.map((i) => i.platform))].sort().map((p) => ({ value: p, label: platformLabel(p) })),
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.upstreamBalance.allStatuses') },
  { value: 'ok', label: t('admin.upstreamBalance.cards.ok') },
  { value: 'failed', label: t('admin.upstreamBalance.cards.failed') },
  { value: 'unsupported', label: t('admin.upstreamBalance.unsupported') },
  { value: 'never', label: t('admin.upstreamBalance.neverChecked') },
])

const visibleItems = computed(() => {
  const kw = filterSearch.value.trim().toLowerCase()
  return items.value.filter((item) => {
    if (!showHidden.value && hiddenIds.value.has(item.account_id)) return false
    if (filterPlatform.value && item.platform !== filterPlatform.value) return false
    if (filterStatus.value) {
      const st = item.snapshot?.status || 'never'
      if (filterStatus.value === 'ok' && st !== 'success') return false
      if (filterStatus.value === 'failed' && st !== 'error') return false
      if (filterStatus.value === 'unsupported' && st !== 'unsupported') return false
      if (filterStatus.value === 'never' && st !== 'never') return false
    }
    if (kw && !`${item.account_name} ${item.account_id} ${item.base_url_host}`.toLowerCase().includes(kw)) return false
    return true
  })
})

// ─── 按上游聚合：同 host 行聚拢，组内首个为头行（+N 徽章展开），其余默认收起 ───
const hostGroupSizes = computed(() => {
  const sizes = new Map<string, number>()
  for (const item of visibleItems.value) {
    const host = item.base_url_host || ''
    sizes.set(host, (sizes.get(host) || 0) + 1)
  }
  return sizes
})

const rowItems = computed(() => {
  if (!groupByUpstream.value) return visibleItems.value
  const order = new Map<string, number>()
  const ranked = [...hostGroupSizes.value.entries()].sort((a, b) => b[1] - a[1])
  ranked.forEach(([host], i) => order.set(host, i))
  return [...visibleItems.value].sort((x, y) => {
    const hx = x.base_url_host || ''
    const hy = y.base_url_host || ''
    if (hx === hy) return 0
    return (order.get(hx) ?? 1e9) - (order.get(hy) ?? 1e9)
  })
})

function hostGroupSize(host: string | undefined): number {
  return hostGroupSizes.value.get(host || '') || 1
}

function isGroupHead(item: UpstreamBalanceListItem): boolean {
  if (!groupByUpstream.value) return false
  if (hostGroupSize(item.base_url_host) <= 1) return false
  return rowItems.value.find((r) => (r.base_url_host || '') === (item.base_url_host || ''))?.account_id === item.account_id
}

function hostExpanded(host: string | undefined): boolean {
  return expandedHosts.value.has(host || '')
}

function toggleHostGroup(host: string | undefined): void {
  const key = host || ''
  const next = new Set(expandedHosts.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedHosts.value = next
}

function rowVisible(item: UpstreamBalanceListItem): boolean {
  if (!groupByUpstream.value) return true
  if (hostGroupSize(item.base_url_host) <= 1) return true
  if (isGroupHead(item)) return true
  return hostExpanded(item.base_url_host)
}

const stats = computed(() => {
  let queryable = 0
  let ok = 0
  let failed = 0
  let total = 0
  for (const item of items.value) {
    if (item.queryable) queryable++
    const st = item.snapshot?.status
    if (st === 'success') {
      ok++
      // 无限额度令牌的余额是 new-api 硬编码的 1 亿，计入合计无意义。
      if (!item.snapshot?.unlimited) total += item.snapshot?.balance || 0
    } else if (st === 'error') {
      failed++
    }
  }
  return { queryable, ok, failed, total }
})

function fmtMoney(v: number | undefined): string {
  return `$${Number(v || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function formatTime(at: string | undefined): string {
  if (!at) return ''
  const d = new Date(at)
  return Number.isNaN(d.getTime()) ? at : d.toLocaleString()
}

function truncateText(s: string | undefined, n: number): string {
  s = (s || '').trim()
  return s.length <= n ? s : s.slice(0, n) + '…'
}

function balanceClass(v: number | undefined): string {
  if (v === undefined) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (v > 5) return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (v > 0) return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
}

async function reload() {
  loading.value = true
  try {
    const res = await getUpstreamBalances()
    items.value = res.items || []
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    loading.value = false
  }
}

async function refreshOne(item: UpstreamBalanceListItem) {
  refreshingIds.value.add(item.account_id)
  refreshingIds.value = new Set(refreshingIds.value)
  try {
    const result = await refreshUpstreamBalance(item.account_id)
    if (result.status === 'success') {
      appStore.showSuccess(t('admin.upstreamBalance.refreshed', { balance: fmtMoney(result.balance) }))
    } else {
      appStore.showError(t('admin.upstreamBalance.refreshFailed', { error: truncateText(result.error, 80) }))
    }
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    refreshingIds.value.delete(item.account_id)
    refreshingIds.value = new Set(refreshingIds.value)
  }
}

async function refreshAll() {
  refreshingAll.value = true
  appStore.showInfo?.(t('admin.upstreamBalance.batchStarted'))
  try {
    const res = await refreshUpstreamBalances()
    const ok = (res.items || []).filter((r) => r.status === 'success').length
    appStore.showSuccess(t('admin.upstreamBalance.batchDone', { ok, total: (res.items || []).length }))
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    refreshingAll.value = false
  }
}

onMounted(reload)
</script>
