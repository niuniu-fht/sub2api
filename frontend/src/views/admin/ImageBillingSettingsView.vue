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
            配置后，图片请求命中该分组的 1K/2K/4K 档位会按多选账号顺序尝试调度；未配置的档位继续使用默认账号调度。
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

      <div class="mt-5 grid gap-3 lg:grid-cols-[1fr_auto] lg:items-end">
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">搜索分组</span>
          <input
            v-model="groupSearch"
            type="text"
            class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            placeholder="输入分组名、ID 或平台"
          />
        </label>
        <div class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-900 dark:text-gray-400">
          显示 {{ filteredSchedulableGroups.length }} / {{ schedulableGroups.length }} 个分组
        </div>
      </div>

      <div class="mt-4 max-h-[70vh] space-y-4 overflow-y-auto pr-2">
        <div
          v-for="group in filteredSchedulableGroups"
          :key="group.id"
          class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/60"
        >
          <div class="flex flex-col gap-2 border-b border-gray-200 pb-3 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <div class="font-medium text-gray-900 dark:text-white">{{ group.name }}</div>
              <div class="text-xs text-gray-500 dark:text-gray-400">#{{ group.id }} · {{ group.platform }}</div>
            </div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              可选账号：<span class="font-medium text-gray-700 dark:text-gray-200">{{ accountOptionsForGroup(group.id).length }}</span>
            </div>
          </div>

          <div class="mt-4 grid gap-3 xl:grid-cols-3">
            <div
              v-for="tier in routingTiers"
              :key="tier.key"
              class="min-w-0 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800"
            >
              <div class="mb-3 flex items-start justify-between gap-2">
                <div>
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ tier.label }} 指定账号</div>
                  <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ selectedRoutingLabel(group.id, tier.key) }}；按上到下优先
                  </div>
                </div>
                <button
                  v-if="routingAccountIDsFor(group.id, tier.key).length > 0"
                  type="button"
                  class="shrink-0 text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200"
                  @click="clearRoutingAccounts(group.id, tier.key)"
                >
                  清空
                </button>
              </div>

              <div class="space-y-3">
                <label class="block">
                  <span class="sr-only">搜索{{ tier.label }}账号</span>
                  <input
                    v-model="accountSearches[tierSearchKey(group.id, tier.key)]"
                    type="text"
                    class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-xs text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                    :placeholder="`搜索 ${tier.label} 可选账号`"
                  />
                </label>

                <div class="rounded-lg border border-primary-100 bg-primary-50/50 p-2 dark:border-primary-900/40 dark:bg-primary-900/10">
                  <div class="mb-2 text-xs font-medium text-primary-700 dark:text-primary-200">已选优先级</div>
                  <div class="max-h-44 space-y-1 overflow-y-auto pr-1">
                    <div
                      v-for="(account, index) in selectedAccountsForTier(group.id, tier.key)"
                      :key="account.id"
                      class="flex items-center gap-2 rounded-lg bg-white px-2 py-1.5 text-xs shadow-sm dark:bg-dark-900"
                    >
                      <span class="shrink-0 rounded-full bg-primary-600 px-2 py-0.5 font-semibold text-white">
                        优先级 {{ index + 1 }}
                      </span>
                      <span class="min-w-0 flex-1 truncate text-gray-800 dark:text-gray-100">{{ accountLabel(account) }}</span>
                      <button
                        type="button"
                        class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
                        :disabled="index === 0"
                        @click="moveRoutingAccount(group.id, tier.key, index, -1)"
                      >
                        上移
                      </button>
                      <button
                        type="button"
                        class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
                        :disabled="index === selectedAccountsForTier(group.id, tier.key).length - 1"
                        @click="moveRoutingAccount(group.id, tier.key, index, 1)"
                      >
                        下移
                      </button>
                      <button
                        type="button"
                        class="rounded border border-red-200 px-1.5 py-0.5 text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-900/20"
                        @click="removeRoutingAccount(group.id, tier.key, account.id)"
                      >
                        移除
                      </button>
                    </div>
                    <div
                      v-if="selectedAccountsForTier(group.id, tier.key).length === 0"
                      class="rounded-lg border border-dashed border-primary-200 px-3 py-4 text-center text-xs text-primary-700/70 dark:border-primary-900/60 dark:text-primary-200/70"
                    >
                      未指定，使用默认调度
                    </div>
                  </div>
                </div>

                <div>
                  <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-300">可添加账号</div>
                  <div class="max-h-52 space-y-1 overflow-y-auto pr-1">
                    <button
                      v-for="account in availableAccountsForTier(group.id, tier.key)"
                      :key="account.id"
                      type="button"
                      class="flex w-full items-start gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition hover:bg-gray-50 dark:hover:bg-dark-900"
                      @click="addRoutingAccount(group.id, tier.key, account.id)"
                    >
                      <span class="mt-0.5 shrink-0 rounded border border-gray-300 px-1.5 py-0.5 text-[11px] text-gray-500 dark:border-dark-600 dark:text-gray-300">添加</span>
                      <span class="min-w-0 flex-1">
                        <span class="block truncate font-medium text-gray-900 dark:text-white">{{ accountLabel(account) }}</span>
                        <span class="block truncate text-gray-500 dark:text-gray-400">
                          {{ String(account.platform || '').toUpperCase() }} · 并发 {{ account.concurrency ?? '-' }}
                        </span>
                      </span>
                    </button>
                    <div
                      v-if="availableAccountsForTier(group.id, tier.key).length === 0"
                      class="rounded-lg border border-dashed border-gray-200 px-3 py-4 text-center text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400"
                    >
                      没有可添加账号
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="!loading && filteredSchedulableGroups.length === 0"
          class="rounded-xl border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
        >
          没有匹配的分组
        </div>
      </div>
      <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
        注意：可选账号仅统计状态 active 且调度开关开启的账号；多选账号会按配置顺序尝试获取可用并发，仍会经过分组归属、模型能力、并发和上游限制检查。
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
import { accountsAPI, groupsAPI } from '@/api/admin'
import {
  getImageBillingAccountRoutingSettings,
  getImageBillingThresholdSettings,
  updateImageBillingAccountRoutingSettings,
  updateImageBillingThresholdSettings
} from '@/api/admin/settings'
import { useAppStore } from '@/stores'
import type { Account, AdminGroup } from '@/types'


const DEFAULT_2K_THRESHOLD = 3000000
const DEFAULT_4K_THRESHOLD = 6000000

const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const savingRouting = ref(false)
const groups = ref<AdminGroup[]>([])
const accounts = ref<Account[]>([])
const groupSearch = ref('')
const accountSearches = reactive<Record<string, string>>({})
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
type ImageBillingRoutingFormRow = {
  one_k_account_ids: number[]
  two_k_account_ids: number[]
  four_k_account_ids: number[]
}

type ImageBillingRoutingTier = 'one_k' | 'two_k' | 'four_k'

const routingTiers: Array<{ key: ImageBillingRoutingTier; label: string }> = [
  { key: 'one_k', label: '1K' },
  { key: 'two_k', label: '2K' },
  { key: 'four_k', label: '4K' },
]

type ImageBillingRoutingSettingsInput = {
  groups?: Record<string, {
    one_k_account_ids?: number[]
    two_k_account_ids?: number[]
    four_k_account_ids?: number[]
    one_k_account_id?: number
    two_k_account_id?: number
    four_k_account_id?: number
  }>
}

type ImageBillingRoutingPayload = {
  groups: Record<string, { one_k_account_ids: number[]; two_k_account_ids: number[]; four_k_account_ids: number[] }>
}

const routingForm = reactive<Record<string, ImageBillingRoutingFormRow>>({})
const schedulableGroups = computed(() =>
  groups.value
    .filter((group) => ['openai', 'composite', 'grok'].includes(String(group.platform || '').toLowerCase()))
    .sort((a, b) => a.id - b.id),
)

const filteredSchedulableGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return schedulableGroups.value
  return schedulableGroups.value.filter((group) => {
    return [group.name, group.platform, group.id]
      .some((value) => String(value ?? '').toLowerCase().includes(keyword))
  })
})

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
      routingForm[key] = { one_k_account_ids: [], two_k_account_ids: [], four_k_account_ids: [] }
    }
  }
}

function normalizeAccountIDs(ids?: number[], legacyID?: number): number[] {
  const seen = new Set<number>()
  const result: number[] = []
  const add = (value: unknown) => {
    const id = Math.trunc(Number(value) || 0)
    if (id <= 0 || seen.has(id)) return
    seen.add(id)
    result.push(id)
  }
  ;(ids || []).forEach(add)
  add(legacyID)
  return result
}

function accountBelongsToGroup(account: Account, groupId: number): boolean {
  if (Array.isArray(account.group_ids) && account.group_ids.includes(groupId)) return true
  return Number((account as any).group_id || 0) === groupId
}

function accountOptionsForGroup(groupId: number): Account[] {
  return accounts.value
    .filter((account) => {
      const platform = String(account.platform || '').toLowerCase()
      if (!['openai', 'grok'].includes(platform)) return false
      if (account.status !== 'active' || account.schedulable !== true) return false
      return accountBelongsToGroup(account, groupId)
    })
    .sort((a, b) => a.id - b.id)
}

function accountLabel(account: Account): string {
  return `#${account.id} ${account.name || '-'}`
}

function tierSearchKey(groupId: number, tier: ImageBillingRoutingTier): string {
  return `${groupId}:${tier}`
}

function selectedAccountsForTier(groupId: number, tier: ImageBillingRoutingTier): Account[] {
  const options = new Map(accountOptionsForGroup(groupId).map((account) => [account.id, account]))
  return routingAccountIDsFor(groupId, tier)
    .map((id) => options.get(id))
    .filter((account): account is Account => Boolean(account))
}

function availableAccountsForTier(groupId: number, tier: ImageBillingRoutingTier): Account[] {
  const selected = new Set(routingAccountIDsFor(groupId, tier))
  const keyword = (accountSearches[tierSearchKey(groupId, tier)] || '').trim().toLowerCase()
  return accountOptionsForGroup(groupId)
    .filter((account) => !selected.has(account.id))
    .filter((account) => {
      if (!keyword) return true
      return [account.id, account.name, account.platform]
        .some((value) => String(value ?? '').toLowerCase().includes(keyword))
    })
}

function enabledAccountIDSetForGroup(groupId: number): Set<number> {
  return new Set(accountOptionsForGroup(groupId).map((account) => account.id))
}

function pruneRoutingRowsToEnabledAccounts(): void {
  for (const group of schedulableGroups.value) {
    const key = String(group.id)
    const row = routingForm[key]
    if (!row) continue
    const enabledIDs = enabledAccountIDSetForGroup(group.id)
    row.one_k_account_ids = normalizeAccountIDs(row.one_k_account_ids).filter((id) => enabledIDs.has(id))
    row.two_k_account_ids = normalizeAccountIDs(row.two_k_account_ids).filter((id) => enabledIDs.has(id))
    row.four_k_account_ids = normalizeAccountIDs(row.four_k_account_ids).filter((id) => enabledIDs.has(id))
  }
}

function routingAccountIDsFor(groupId: number, tier: ImageBillingRoutingTier): number[] {
  const key = String(groupId)
  const row = routingForm[key]
  if (!row) return []
  if (tier === 'one_k') return row.one_k_account_ids
  return tier === 'two_k' ? row.two_k_account_ids : row.four_k_account_ids
}

function selectedRoutingLabel(groupId: number, tier: ImageBillingRoutingTier): string {
  const count = routingAccountIDsFor(groupId, tier).length
  return count > 0 ? `已选 ${count} 个账号` : '默认调度'
}

function toggleRoutingAccount(groupId: number, tier: ImageBillingRoutingTier, accountId: number, checked: boolean): void {
  const key = String(groupId)
  if (!routingForm[key]) {
    routingForm[key] = { one_k_account_ids: [], two_k_account_ids: [], four_k_account_ids: [] }
  }
  const list = tier === 'one_k' ? routingForm[key].one_k_account_ids : tier === 'two_k' ? routingForm[key].two_k_account_ids : routingForm[key].four_k_account_ids
  const id = Math.trunc(Number(accountId) || 0)
  const index = list.indexOf(id)
  if (checked && id > 0 && index === -1) {
    list.push(id)
  } else if (!checked && index !== -1) {
    list.splice(index, 1)
  }
}

function addRoutingAccount(groupId: number, tier: ImageBillingRoutingTier, accountId: number): void {
  toggleRoutingAccount(groupId, tier, accountId, true)
}

function removeRoutingAccount(groupId: number, tier: ImageBillingRoutingTier, accountId: number): void {
  toggleRoutingAccount(groupId, tier, accountId, false)
}

function moveRoutingAccount(groupId: number, tier: ImageBillingRoutingTier, index: number, direction: -1 | 1): void {
  const list = routingAccountIDsFor(groupId, tier)
  const target = index + direction
  if (index < 0 || target < 0 || index >= list.length || target >= list.length) return
  const [item] = list.splice(index, 1)
  list.splice(target, 0, item)
}

function clearRoutingAccounts(groupId: number, tier: ImageBillingRoutingTier): void {
  const key = String(groupId)
  if (!routingForm[key]) return
  if (tier === 'one_k') {
    routingForm[key].one_k_account_ids = []
  } else if (tier === 'two_k') {
    routingForm[key].two_k_account_ids = []
  } else {
    routingForm[key].four_k_account_ids = []
  }
}

function applyRoutingSettings(settings: ImageBillingRoutingSettingsInput): void {
  Object.keys(routingForm).forEach((key) => delete routingForm[key])
  const rows = settings.groups || {}
  for (const [groupId, routing] of Object.entries(rows)) {
    routingForm[groupId] = {
      one_k_account_ids: normalizeAccountIDs(routing.one_k_account_ids, routing.one_k_account_id),
      two_k_account_ids: normalizeAccountIDs(routing.two_k_account_ids, routing.two_k_account_id),
      four_k_account_ids: normalizeAccountIDs(routing.four_k_account_ids, routing.four_k_account_id),
    }
  }
  ensureRoutingRows()
  pruneRoutingRowsToEnabledAccounts()
}

function buildRoutingPayload(): ImageBillingRoutingPayload {
  const payload: ImageBillingRoutingPayload['groups'] = {}
  for (const [groupId, routing] of Object.entries(routingForm)) {
    const groupIDNumber = Math.trunc(Number(groupId) || 0)
    const enabledIDs = enabledAccountIDSetForGroup(groupIDNumber)
    const oneK = normalizeAccountIDs(routing.one_k_account_ids).filter((id) => enabledIDs.has(id))
    const twoK = normalizeAccountIDs(routing.two_k_account_ids).filter((id) => enabledIDs.has(id))
    const fourK = normalizeAccountIDs(routing.four_k_account_ids).filter((id) => enabledIDs.has(id))
    if (oneK.length > 0 || twoK.length > 0 || fourK.length > 0) {
      payload[groupId] = { one_k_account_ids: oneK, two_k_account_ids: twoK, four_k_account_ids: fourK }
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

