<template>
  <AppLayout>
  <div class="mx-auto max-w-5xl space-y-6">
    <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">图片计费档位配置</h1>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            按 width * height 的总像素判断图片计费档位：大于 2K 阈值计 2K，大于 4K 阈值计 4K，否则计 1K。
          </p>
        </div>
        <button
          type="button"
          class="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
          :disabled="loading || saving"
          @click="loadAllSettings"
        >
          刷新
        </button>
      </div>
    </div>

    <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <div class="grid gap-5 md:grid-cols-2">
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">2K 像素阈值（不含）</span>
          <input
            v-model.number="form.two_k_pixel_threshold"
            type="number"
            min="1"
            step="1"
            class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            placeholder="3000000"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">默认 3,000,000；总像素大于该值进入 2K。</p>
        </label>

        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">4K 像素阈值（不含）</span>
          <input
            v-model.number="form.four_k_pixel_threshold"
            type="number"
            min="1"
            step="1"
            class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            placeholder="6000000"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">默认 6,000,000；总像素大于该值进入 4K。</p>
        </label>
      </div>

      <div class="mt-6 flex flex-wrap gap-3">
        <button
          type="button"
          class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60"
          :disabled="saving || loading"
          @click="saveSettings"
        >
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
        <button
          type="button"
          class="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
          :disabled="saving"
          @click="resetDefaults"
        >
          恢复默认值
        </button>
      </div>
    </div>

    <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">分组图片档位账号调度</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            配置后，图片请求命中该分组的 2K/4K 档位会强制走指定账号；未配置的档位继续使用默认账号调度。
          </p>
        </div>
        <button
          type="button"
          class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60"
          :disabled="loading || savingRouting"
          @click="saveRoutingSettings"
        >
          {{ savingRouting ? '保存中...' : '保存调度配置' }}
        </button>
      </div>

      <div class="mt-5 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-900">
            <tr>
              <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">分组</th>
              <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">2K 指定账号</th>
              <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">4K 指定账号</th>
              <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">可选账号数</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
            <tr v-for="group in schedulableGroups" :key="group.id">
              <td class="px-4 py-3">
                <div class="font-medium text-gray-900 dark:text-white">{{ group.name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">#{{ group.id }} · {{ group.platform }}</div>
              </td>
              <td class="px-4 py-3">
                <Select
                  v-model="routingForm[String(group.id)].two_k_account_id"
                  :options="accountSelectOptionsForGroup(group.id)"
                  :searchable="accountOptionsForGroup(group.id).length > 6"
                  empty-text="无可用账号"
                  aria-label="2K 指定账号"
                >
                  <template #selected="{ option }">
                    <span class="flex min-w-0 items-center gap-2">
                      <span
                        class="h-2 w-2 flex-shrink-0 rounded-full"
                        :class="option?.isDefault ? 'bg-gray-400' : option?.status === 'active' ? 'bg-emerald-500' : option?.status === 'error' ? 'bg-red-500' : 'bg-amber-400'"
                      />
                      <span class="truncate">{{ option?.label || '默认调度' }}</span>
                    </span>
                  </template>
                  <template #option="{ option }">
                    <div class="flex min-w-0 flex-1 items-center justify-between gap-3">
                      <div class="min-w-0">
                        <div class="truncate font-medium text-gray-900 dark:text-white">{{ option.label }}</div>
                        <div v-if="!option.isDefault" class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
                          {{ option.platform }} · 并发 {{ option.concurrency ?? '-' }}
                        </div>
                        <div v-else class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                          使用当前分组默认调度策略
                        </div>
                      </div>
                      <span
                        class="flex-shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
                        :class="option.isDefault ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' : option.status === 'active' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300' : option.status === 'error' ? 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'"
                      >
                        {{ option.isDefault ? '默认' : option.status }}
                      </span>
                    </div>
                  </template>
                </Select>
              </td>
              <td class="px-4 py-3">
                <Select
                  v-model="routingForm[String(group.id)].four_k_account_id"
                  :options="accountSelectOptionsForGroup(group.id)"
                  :searchable="accountOptionsForGroup(group.id).length > 6"
                  empty-text="无可用账号"
                  aria-label="4K 指定账号"
                >
                  <template #selected="{ option }">
                    <span class="flex min-w-0 items-center gap-2">
                      <span
                        class="h-2 w-2 flex-shrink-0 rounded-full"
                        :class="option?.isDefault ? 'bg-gray-400' : option?.status === 'active' ? 'bg-emerald-500' : option?.status === 'error' ? 'bg-red-500' : 'bg-amber-400'"
                      />
                      <span class="truncate">{{ option?.label || '默认调度' }}</span>
                    </span>
                  </template>
                  <template #option="{ option }">
                    <div class="flex min-w-0 flex-1 items-center justify-between gap-3">
                      <div class="min-w-0">
                        <div class="truncate font-medium text-gray-900 dark:text-white">{{ option.label }}</div>
                        <div v-if="!option.isDefault" class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
                          {{ option.platform }} · 并发 {{ option.concurrency ?? '-' }}
                        </div>
                        <div v-else class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                          使用当前分组默认调度策略
                        </div>
                      </div>
                      <span
                        class="flex-shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
                        :class="option.isDefault ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' : option.status === 'active' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300' : option.status === 'error' ? 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'"
                      >
                        {{ option.isDefault ? '默认' : option.status }}
                      </span>
                    </div>
                  </template>
                </Select>
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                {{ accountOptionsForGroup(group.id).length }}
              </td>
            </tr>
            <tr v-if="!loading && schedulableGroups.length === 0">
              <td colspan="4" class="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                暂无可配置分组
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
        注意：强制账号仍会经过账号状态、分组归属、模型能力、并发和上游限制检查；账号不可用时，该档位请求会返回无可用账号，不再绕回默认调度。
      </p>
    </div>

    <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">快速验证</h2>
      <div class="mt-4 grid gap-4 md:grid-cols-3">
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">width</span>
          <input v-model.number="sample.width" type="number" min="1" step="1" class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
        </label>
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">height</span>
          <input v-model.number="sample.height" type="number" min="1" step="1" class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
        </label>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-sm text-gray-500 dark:text-gray-400">当前样例</div>
          <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ sample.width }}×{{ sample.height }} → {{ sampleTier }}</div>
          <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">总像素：{{ formatNumber(sampleArea) }}</div>
        </div>
      </div>

      <div class="mt-5 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-900">
            <tr>
              <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">档位</th>
              <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">判断规则</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
            <tr>
              <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">1K</td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">width * height ≤ {{ formatNumber(form.two_k_pixel_threshold) }}</td>
            </tr>
            <tr>
              <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">2K</td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ formatNumber(form.two_k_pixel_threshold) }} &lt; width * height ≤ {{ formatNumber(form.four_k_pixel_threshold) }}</td>
            </tr>
            <tr>
              <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">4K</td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">width * height &gt; {{ formatNumber(form.four_k_pixel_threshold) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import { accountsAPI, groupsAPI } from '@/api/admin'
import {
  getImageBillingAccountRoutingSettings,
  getImageBillingThresholdSettings,
  updateImageBillingAccountRoutingSettings,
  updateImageBillingThresholdSettings
} from '@/api/admin/settings'
import { useAppStore } from '@/stores'
import type { Account, AdminGroup } from '@/types'

interface AccountSelectOptionItem {
  [key: string]: unknown
  value: number
  label: string
  name?: string
  platform?: string
  status?: string
  concurrency?: number
  isDefault?: boolean
}

const DEFAULT_2K_THRESHOLD = 3000000
const DEFAULT_4K_THRESHOLD = 6000000

const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const savingRouting = ref(false)
const groups = ref<AdminGroup[]>([])
const accounts = ref<Account[]>([])
const lastAutoReloadAt = ref(0)

const form = reactive({
  two_k_pixel_threshold: DEFAULT_2K_THRESHOLD,
  four_k_pixel_threshold: DEFAULT_4K_THRESHOLD,
})

const sample = reactive({
  width: 1536,
  height: 2048,
})

const sampleArea = computed(() => Math.max(0, Number(sample.width) || 0) * Math.max(0, Number(sample.height) || 0))
const sampleTier = computed(() => {
  if (sampleArea.value > form.four_k_pixel_threshold) return '4K'
  if (sampleArea.value > form.two_k_pixel_threshold) return '2K'
  return '1K'
})
const routingForm = reactive<Record<string, { two_k_account_id: number; four_k_account_id: number }>>({})
const schedulableGroups = computed(() =>
  groups.value
    .filter((group) => ['openai', 'composite', 'grok'].includes(String(group.platform || '').toLowerCase()))
    .sort((a, b) => a.id - b.id),
)

function formatNumber(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(Math.trunc(Number(value) || 0))
}

function validateForm(): boolean {
  form.two_k_pixel_threshold = Math.trunc(Number(form.two_k_pixel_threshold) || 0)
  form.four_k_pixel_threshold = Math.trunc(Number(form.four_k_pixel_threshold) || 0)
  if (form.two_k_pixel_threshold <= 0 || form.four_k_pixel_threshold <= 0) {
    appStore.showError('阈值必须大于 0')
    return false
  }
  if (form.four_k_pixel_threshold <= form.two_k_pixel_threshold) {
    appStore.showError('4K 阈值必须大于 2K 阈值')
    return false
  }
  return true
}

async function saveSettings(): Promise<void> {
  if (!validateForm()) return
  saving.value = true
  try {
    const settings = await updateImageBillingThresholdSettings({ ...form })
    form.two_k_pixel_threshold = settings.two_k_pixel_threshold
    form.four_k_pixel_threshold = settings.four_k_pixel_threshold
    appStore.showSuccess('图片计费配置已保存')
  } catch (error: any) {
    appStore.showError(error?.message || '保存图片计费配置失败')
  } finally {
    saving.value = false
  }
}

function resetDefaults(): void {
  form.two_k_pixel_threshold = DEFAULT_2K_THRESHOLD
  form.four_k_pixel_threshold = DEFAULT_4K_THRESHOLD
}

function ensureRoutingRows(): void {
  for (const group of schedulableGroups.value) {
    const key = String(group.id)
    if (!routingForm[key]) {
      routingForm[key] = { two_k_account_id: 0, four_k_account_id: 0 }
    }
  }
}

function accountOptionsForGroup(groupId: number): Account[] {
  return accounts.value
    .filter((account) => {
      const platform = String(account.platform || '').toLowerCase()
      if (!['openai', 'grok'].includes(platform)) return false
      if (Array.isArray(account.group_ids) && account.group_ids.includes(groupId)) return true
      return Number((account as any).group_id || 0) === groupId
    })
    .sort((a, b) => a.id - b.id)
}

function accountLabel(account: Account): string {
  return `#${account.id} ${account.name || '-'} (${account.status})`
}

function accountSelectOptionsForGroup(groupId: number): AccountSelectOptionItem[] {
  return [
    {
      value: 0,
      label: '默认调度',
      isDefault: true,
    },
    ...accountOptionsForGroup(groupId).map((account) => ({
      value: account.id,
      label: accountLabel(account),
      name: account.name || '-',
      platform: String(account.platform || '').toUpperCase(),
      status: account.status,
      concurrency: account.concurrency,
    })),
  ]
}

function applyRoutingSettings(settings: { groups?: Record<string, { two_k_account_id?: number; four_k_account_id?: number }> }): void {
  Object.keys(routingForm).forEach((key) => delete routingForm[key])
  const rows = settings.groups || {}
  for (const [groupId, routing] of Object.entries(rows)) {
    routingForm[groupId] = {
      two_k_account_id: Number(routing.two_k_account_id || 0),
      four_k_account_id: Number(routing.four_k_account_id || 0),
    }
  }
  ensureRoutingRows()
}

function buildRoutingPayload(): { groups: Record<string, { two_k_account_id: number; four_k_account_id: number }> } {
  const payload: Record<string, { two_k_account_id: number; four_k_account_id: number }> = {}
  for (const [groupId, routing] of Object.entries(routingForm)) {
    const twoK = Math.trunc(Number(routing.two_k_account_id) || 0)
    const fourK = Math.trunc(Number(routing.four_k_account_id) || 0)
    if (twoK > 0 || fourK > 0) {
      payload[groupId] = { two_k_account_id: Math.max(0, twoK), four_k_account_id: Math.max(0, fourK) }
    }
  }
  return { groups: payload }
}

async function loadRoutingAccounts(): Promise<Account[]> {
  // 后端 page_size 上限是 1000；传 10000 会被回退成默认 20，导致新加账号不出现在调度下拉框。
  const pageSize = 1000
  const firstPage = await accountsAPI.list(1, pageSize, { lite: 'true' })
  const allAccounts = [...(firstPage.items || [])]
  const totalPages = Math.max(1, Number(firstPage.pages) || 1)

  for (let page = 2; page <= totalPages; page += 1) {
    const nextPage = await accountsAPI.list(page, pageSize, { lite: 'true' })
    allAccounts.push(...(nextPage.items || []))
  }

  return allAccounts
}

async function loadAllSettings(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    const [thresholds, groupItems, accountItems, routing] = await Promise.all([
      getImageBillingThresholdSettings(),
      groupsAPI.getAllIncludingInactive(),
      loadRoutingAccounts(),
      getImageBillingAccountRoutingSettings(),
    ])
    form.two_k_pixel_threshold = thresholds.two_k_pixel_threshold || DEFAULT_2K_THRESHOLD
    form.four_k_pixel_threshold = thresholds.four_k_pixel_threshold || DEFAULT_4K_THRESHOLD
    groups.value = groupItems || []
    accounts.value = accountItems || []
    applyRoutingSettings(routing)
  } catch (error: any) {
    appStore.showError(error?.message || '加载图片计费配置失败')
  } finally {
    loading.value = false
  }
}

function autoReloadSettings(): void {
  if (saving.value || savingRouting.value) return
  const now = Date.now()
  if (now - lastAutoReloadAt.value < 1500) return
  lastAutoReloadAt.value = now
  void loadAllSettings()
}

function handleWindowFocus(): void {
  autoReloadSettings()
}

function handleVisibilityChange(): void {
  if (document.visibilityState === 'visible') {
    autoReloadSettings()
  }
}

async function saveRoutingSettings(): Promise<void> {
  savingRouting.value = true
  try {
    const settings = await updateImageBillingAccountRoutingSettings(buildRoutingPayload())
    applyRoutingSettings(settings)
    appStore.showSuccess('图片档位账号调度配置已保存')
  } catch (error: any) {
    appStore.showError(error?.message || '保存图片档位账号调度配置失败')
  } finally {
    savingRouting.value = false
  }
}

onMounted(() => {
  void loadAllSettings()
  window.addEventListener('focus', handleWindowFocus)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onBeforeUnmount(() => {
  window.removeEventListener('focus', handleWindowFocus)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
