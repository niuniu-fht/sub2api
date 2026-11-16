<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6">
      <!-- 页头 -->
      <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.costTitle') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.profit.costDescription') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-primary btn-sm" @click="openPicker">
              {{ t('admin.profit.cost.addAccount') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">
              {{ t('admin.profit.refresh') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 说明 -->
      <div class="rounded-xl border border-gray-200 bg-white px-4 py-3 text-xs text-gray-500 shadow-sm dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        {{ t('admin.profit.rules.hint') }}
      </div>

      <!-- 账号分组表 -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <!-- 过滤器：平台 chips + 分组 + 搜索 -->
        <div class="mb-4 flex flex-wrap items-center gap-2">
          <div class="flex flex-wrap gap-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-900">
            <button
              v-for="opt in platformFilterOptions"
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
          <Select
            v-model="filterGroup"
            :options="groupSelectOptions"
            class="w-48"
          />
          <SearchInput
            v-model="filterSearch"
            :placeholder="t('admin.profit.cost.searchAccount')"
            class="ml-auto w-48"
          />
        </div>

        <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
                <th class="px-3 py-2.5">{{ t('admin.profit.cost.col.account') }}</th>
                <th class="px-3 py-2.5">{{ t('admin.profit.cost.col.groups') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('admin.profit.cost.col.models') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('admin.profit.cost.col.rules') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('admin.profit.cost.col.updated') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
              <template v-for="group in visibleGroups" :key="group.key">
                <tr
                  class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
                  :data-testid="`cost-group-${group.key}`"
                  @click="toggleGroup(group)"
                >
                  <td class="px-3 py-2.5">
                    <div class="flex items-center gap-2">
                      <svg
                        class="h-3.5 w-3.5 shrink-0 text-gray-400 transition-transform duration-200"
                        :class="expanded[group.key] ? 'rotate-90' : ''"
                        fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
                      >
                        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                      </svg>
                      <span
                        v-if="group.accountId === null"
                        class="inline-flex items-center gap-1.5 rounded-md bg-primary-50 px-2 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200"
                      >
                        <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16" />
                        </svg>
                        {{ t('admin.profit.rules.anyAccount') }}
                      </span>
                      <template v-else>
                        <span :class="['inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1', platformBadgeClass(group.platform)]">
                          <PlatformIcon :platform="group.platform" size="sm" />
                        </span>
                        <span class="min-w-0 max-w-[280px] truncate font-medium text-gray-900 dark:text-white">
                          {{ group.name || `#${group.accountId}` }}
                        </span>
                        <span class="shrink-0 text-xs text-gray-400">#{{ group.accountId }}</span>
                      </template>
                    </div>
                  </td>
                  <td class="px-3 py-2.5">
                    <div v-if="group.accountId === null" class="text-xs text-gray-400">—</div>
                    <div v-else class="flex max-w-[240px] flex-wrap items-center gap-1">
                      <span
                        v-for="gname in groupNamesFor(group.accountId)"
                        :key="gname"
                        class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                      >
                        {{ gname }}
                      </span>
                      <span v-if="!groupNamesFor(group.accountId).length" class="text-xs text-gray-400">—</span>
                    </div>
                  </td>
                  <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ group.modelsConfigured }}</td>
                  <td class="px-3 py-2.5 text-right tabular-nums" :class="group.rules.length > 0 ? 'text-gray-600 dark:text-gray-300' : 'text-gray-400'">{{ group.rules.length }}</td>
                  <td class="px-3 py-2.5 text-right text-xs text-gray-500 dark:text-gray-400">{{ group.updatedAt || '—' }}</td>
                </tr>

                <!-- 展开区：该账号的模型 × 价格矩阵 -->
                <tr v-if="expanded[group.key]">
                  <td colspan="5" class="bg-gray-50/60 px-3 py-3 dark:bg-dark-900/40">
                    <div v-if="editorFor(group).loading" class="py-8 text-center text-sm text-gray-400">{{ t('common.loading') }}</div>
                    <template v-else>
                      <CostRuleMatrix :rows="editorFor(group).rows" />

                      <!-- 底部操作 -->
                      <div class="mt-2 flex flex-wrap items-center justify-end gap-2">
                        <button
                          v-if="group.rules.length"
                          type="button"
                          class="text-xs font-medium text-red-600 hover:underline dark:text-red-400"
                          @click.stop="confirmClear(group)"
                        >
                          {{ t('admin.profit.cost.clearAll') }}
                        </button>
                        <button
                          type="button"
                          class="btn btn-primary btn-sm"
                          :disabled="editorFor(group).saving"
                          @click.stop="saveGroup(group)"
                        >
                          {{ editorFor(group).saving ? t('common.loading') : t('admin.profit.cost.saveGroup') }}
                        </button>
                      </div>
                    </template>
                  </td>
                </tr>
              </template>
              <tr v-if="!visibleGroups.length && !loading">
                <td colspan="5" class="px-3 py-10 text-center text-gray-400">{{ t('admin.profit.cost.emptyGroups') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- 新增账号配置弹窗 -->
    <BaseDialog
      :show="pickerOpen"
      :title="t('admin.profit.cost.pickerTitle')"
      width="wide"
      :close-on-click-outside="false"
      @close="pickerOpen = false"
    >
      <div class="space-y-3">
        <!-- 过滤器 -->
        <div class="flex flex-wrap items-center gap-2">
          <div class="flex flex-wrap gap-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-900">
            <button
              v-for="opt in pickerPlatformOptions"
              :key="opt.value"
              type="button"
              class="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition"
              :class="pickerPlatform === opt.value
                ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="pickerPlatform = opt.value"
            >
              <PlatformIcon v-if="opt.value" :platform="opt.value" size="xs" />
              <span>{{ opt.label }}</span>
            </button>
          </div>
          <Select
            v-model="pickerGroup"
            :options="groupSelectOptions"
            class="w-44"
          />
          <SearchInput
            v-model="pickerSearch"
            :placeholder="t('admin.profit.cost.searchAccount')"
            class="ml-auto w-44"
          />
        </div>

        <!-- 账号列表：固定高度，切换平台过滤时不抖动 -->
        <div class="h-[50vh] overflow-y-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="min-w-full text-sm">
            <thead class="sticky top-0 bg-gray-50 dark:bg-dark-900">
              <tr class="border-b border-gray-200 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2">{{ t('admin.profit.cost.col.account') }}</th>
                <th class="px-3 py-2">{{ t('admin.profit.cost.col.groups') }}</th>
                <th class="px-3 py-2 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="acc in pickerAccounts"
                :key="acc.id"
                class="border-b border-gray-100 transition dark:border-dark-700/50"
                :class="isConfigured(acc.id)
                  ? 'cursor-not-allowed opacity-50'
                  : 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/40'"
                @click="pickAccount(acc)"
              >
                <td class="px-3 py-2.5">
                  <div class="flex items-center gap-2">
                    <span :class="['inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1', platformBadgeClass(acc.platform)]">
                      <PlatformIcon :platform="acc.platform" size="sm" />
                      <span class="text-xs font-medium">{{ platformLabel(acc.platform) }}</span>
                    </span>
                    <span class="min-w-0 max-w-[300px] truncate font-medium text-gray-900 dark:text-white">{{ acc.name || `#${acc.id}` }}</span>
                    <span class="shrink-0 text-xs text-gray-400">#{{ acc.id }}</span>
                  </div>
                </td>
                <td class="px-3 py-2.5">
                  <div class="flex max-w-[240px] flex-wrap items-center gap-1">
                    <span
                      v-for="gname in groupNamesFor(acc.id)"
                      :key="gname"
                      class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                    >
                      {{ gname }}
                    </span>
                    <span v-if="!groupNamesFor(acc.id).length" class="text-xs text-gray-400">—</span>
                  </div>
                </td>
                <td class="px-3 py-2.5 text-right">
                  <span v-if="isConfigured(acc.id)" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-400 dark:bg-dark-700">
                    {{ t('admin.profit.cost.configured') }}
                  </span>
                  <button
                    v-else
                    type="button"
                    class="btn btn-primary btn-sm"
                    @click.stop="pickAccount(acc)"
                  >
                    {{ t('admin.profit.cost.configure') }}
                  </button>
                </td>
              </tr>
              <tr v-if="!pickerAccounts.length">
                <td colspan="3" class="px-3 py-8 text-center text-gray-400">{{ t('admin.profit.cost.allConfigured') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary btn-sm" @click="pickerOpen = false">{{ t('common.close') }}</button>
      </template>
    </BaseDialog>

    <!-- 配置弹窗：选一次账号，在弹窗内配完价格，确认后才加入账号价格配置 -->
    <BaseDialog
      :show="configOpen"
      :title="t('admin.profit.cost.configTitle')"
      width="wide"
      :close-on-click-outside="false"
      @close="configOpen = false"
    >
      <div class="space-y-3">
        <div v-if="pickTarget" class="flex flex-wrap items-center gap-2 rounded-xl bg-gray-50 px-3 py-2.5 dark:bg-dark-900/60">
          <span :class="['inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1', platformBadgeClass(pickTarget.platform)]">
            <PlatformIcon :platform="pickTarget.platform" size="sm" />
            <span class="text-xs font-medium">{{ platformLabel(pickTarget.platform) }}</span>
          </span>
          <span class="min-w-0 max-w-[320px] truncate font-medium text-gray-900 dark:text-white">{{ pickTarget.name || `#${pickTarget.id}` }}</span>
          <span class="shrink-0 text-xs text-gray-400">#{{ pickTarget.id }}</span>
          <div class="ml-auto flex flex-wrap items-center gap-1">
            <span
              v-for="gname in groupNamesFor(pickTarget.id)"
              :key="gname"
              class="rounded bg-white px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300"
            >
              {{ gname }}
            </span>
          </div>
        </div>
        <div v-if="configLoading" class="py-10 text-center text-sm text-gray-400">{{ t('common.loading') }}</div>
        <template v-else>
          <CostRuleMatrix :rows="configRows" />
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.profit.cost.configHint') }}</p>
        </template>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary btn-sm" @click="configOpen = false">{{ t('common.cancel') }}</button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="configLoading || configSaving"
          @click="confirmConfig"
        >
          {{ configSaving ? t('common.loading') : t('admin.profit.cost.confirmAdd') }}
        </button>
      </template>
    </BaseDialog>

    <!-- 清空确认 -->
    <ConfirmDialog
      :show="clearConfirmShow"
      :title="t('admin.profit.cost.clearAll')"
      :message="t('admin.profit.cost.clearConfirm', { account: clearTargetName })"
      danger
      @confirm="doClearGroup"
      @cancel="clearConfirmShow = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import CostRuleMatrix, { type CostRuleMatrixRow } from '@/components/admin/profit/CostRuleMatrix.vue'
import { useAppStore } from '@/stores/app'
import { accountsAPI } from '@/api/admin/accounts'
import { groupsAPI } from '@/api/admin/groups'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import {
  getCostRules,
  getCostRuleModels,
  replaceCostRules,
  type BatchCostRuleRow,
  type ProfitCostRule,
} from '@/api/admin/profit'

const { t } = useI18n()
const appStore = useAppStore()

interface GroupEditor {
  rows: CostRuleMatrixRow[]
  loading: boolean
  saving: boolean
  loaded: boolean
}

interface AccountGroup {
  key: string
  accountId: number | null
  name: string
  platform: string
  rules: ProfitCostRule[]
  modelsConfigured: number
  updatedAt: string
}

interface AccountOption {
  id: number
  name: string
  platform: string
  groupIDs: number[]
}

const loading = ref(false)
const costRules = ref<ProfitCostRule[]>([])
const accountOptions = ref<AccountOption[]>([])
const groupOptions = ref<{ id: number; name: string }[]>([])
// 手动新增（尚无规则）的账号，保证分组表能立即出现
const manualAccountIds = ref<(number | null)[]>([])
const expanded = reactive<Record<string, boolean>>({})
const editors = reactive<Record<string, GroupEditor>>({})

// 主列表过滤器
const filterPlatform = ref('')
const filterGroup = ref('')
const filterSearch = ref('')

// 新增弹窗状态
const pickerOpen = ref(false)
const pickerPlatform = ref('')
const pickerGroup = ref('')
const pickerSearch = ref('')
// 配置弹窗：选一次账号 → 弹窗内配完价格 → 确认后才加入
const configOpen = ref(false)
const configLoading = ref(false)
const configSaving = ref(false)
const pickTarget = ref<AccountOption | null>(null)
const configRows = ref<CostRuleMatrixRow[]>([])

// 分组下拉选项（Select 组件：选项 >5 自动启用搜索）
const groupSelectOptions = computed(() => [
  { value: '', label: t('admin.profit.cost.allGroups') },
  ...groupOptions.value.map((g) => ({ value: String(g.id), label: g.name })),
])

const clearConfirmShow = ref(false)
const clearTargetKey = ref('')

function groupKey(accountId: number | null): string {
  return accountId === null ? 'any' : String(accountId)
}

// ─── 平台过滤选项（按实际存在的平台生成）───
const presentPlatforms = computed(() => {
  const set = new Set<string>()
  for (const a of accountOptions.value) set.add(a.platform)
  return [...set].sort()
})

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.profit.cost.allPlatforms') },
  ...presentPlatforms.value.map((p) => ({ value: p, label: platformLabel(p) })),
])

const pickerPlatformOptions = platformFilterOptions

// ─── 分组视图：规则按账号聚合，「任意账号」置顶，其余按名称排序 ───
const groups = computed<AccountGroup[]>(() => {
  const byAccount = new Map<string, ProfitCostRule[]>()
  for (const rule of costRules.value) {
    const key = groupKey(rule.account_id ?? null)
    const list = byAccount.get(key) || []
    list.push(rule)
    byAccount.set(key, list)
  }
  for (const id of manualAccountIds.value) {
    const key = groupKey(id)
    if (!byAccount.has(key)) byAccount.set(key, [])
  }

  const result: AccountGroup[] = []
  for (const [key, rules] of byAccount) {
    const accountId = key === 'any' ? null : Number(key)
    const info = accountId === null ? undefined : accountOptions.value.find((a) => a.id === accountId)
    const updated = rules
      .map((r) => r.updated_at || '')
      .sort()
      .pop() || ''
    result.push({
      key,
      accountId,
      name: info?.name || rules[0]?.account_name || (accountId === null ? '' : `#${accountId}`),
      platform: info?.platform || '',
      rules,
      modelsConfigured: rules.length,
      updatedAt: updated ? new Date(updated).toLocaleString() : '',
    })
  }

  result.sort((a, b) => {
    if (a.accountId === null) return -1
    if (b.accountId === null) return 1
    return a.name.localeCompare(b.name)
  })
  return result
})

// ─── 主列表过滤 ───
const visibleGroups = computed(() => {
  const kw = filterSearch.value.trim().toLowerCase()
  return groups.value.filter((g) => {
    if (g.accountId === null) {
      // 「任意账号」仅在无平台/分组过滤时显示
      return !filterPlatform.value && !filterGroup.value && !kw
    }
    if (filterPlatform.value && g.platform !== filterPlatform.value) return false
    if (filterGroup.value) {
      const gid = Number(filterGroup.value)
      const acc = accountOptions.value.find((a) => a.id === g.accountId)
      if (!acc || !acc.groupIDs.includes(gid)) return false
    }
    if (kw && !(`${g.name} ${g.accountId}`.toLowerCase().includes(kw))) return false
    return true
  })
})

// ─── 新增弹窗账号列表（未配置的可调度账号）───
const pickerAccounts = computed(() => {
  const kw = pickerSearch.value.trim().toLowerCase()
  return accountOptions.value.filter((a) => {
    if (pickerPlatform.value && a.platform !== pickerPlatform.value) return false
    if (pickerGroup.value) {
      const gid = Number(pickerGroup.value)
      if (!a.groupIDs.includes(gid)) return false
    }
    if (kw && !(`${a.name} ${a.id}`.toLowerCase().includes(kw))) return false
    return true
  })
})

function isConfigured(accountId: number): boolean {
  return groups.value.some((g) => g.accountId === accountId && g.rules.length > 0)
}

function groupNamesFor(accountId: number): string[] {
  const acc = accountOptions.value.find((a) => a.id === accountId)
  if (!acc) return []
  const byID = new Map(groupOptions.value.map((g) => [g.id, g.name]))
  return acc.groupIDs.map((id) => byID.get(id)).filter((n): n is string => Boolean(n))
}

async function loadCostRules() {
  const res = await getCostRules()
  costRules.value = res.items
}

async function loadAccountOptions() {
  try {
    const res: any = await accountsAPI.list(1, 500, {})
    const items: any[] = res?.items || []
    accountOptions.value = items
      .filter((a: any) => a.schedulable === true && a.status === 'active')
      .map((a: any) => ({ id: a.id, name: a.name || '', platform: a.platform, groupIDs: (a.group_ids as number[]) || [] }))
  } catch {
    accountOptions.value = []
  }
}

async function loadGroupOptions() {
  try {
    const items = await groupsAPI.getAll()
    groupOptions.value = items.map((g: any) => ({ id: g.id, name: g.name }))
  } catch {
    groupOptions.value = []
  }
}

async function reload() {
  loading.value = true
  try {
    await loadCostRules()
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    loading.value = false
  }
}

// ─── 展开/收起：首次展开懒加载该账号的模型矩阵 ───
function editorFor(group: AccountGroup): GroupEditor {
  if (!editors[group.key]) {
    editors[group.key] = { rows: [], loading: false, saving: false, loaded: false }
  }
  return editors[group.key]
}

async function toggleGroup(group: AccountGroup) {
  const next = !expanded[group.key]
  expanded[group.key] = next
  if (next) await loadEditor(group)
}

async function loadEditor(group: AccountGroup) {
  const editor = editorFor(group)
  if (editor.loaded) return
  editor.loading = true
  try {
    const modelRes = await getCostRuleModels(group.accountId)
    const existing = new Map(group.rules.map((r) => [r.model, r]))
    const seen = new Set<string>()
    editor.rows = modelRes.items.map((model) => {
      seen.add(model)
      const r = existing.get(model)
      return {
        model,
        selected: false,
        billing_mode: r?.billing_mode || 'per_image',
        image_price: r?.image_price || 0,
        request_price: r?.request_price || 0,
        input_price: r?.input_price || 0,
        output_price: r?.output_price || 0,
        note: r?.note || '',
      }
    })
    // 已有规则但不在模型列表里的（如历史模型）也带上
    for (const [model, r] of existing) {
      if (!seen.has(model) && model) {
        editor.rows.push({
          model,
          selected: false,
          billing_mode: r.billing_mode,
          image_price: r.image_price,
          request_price: r.request_price,
          input_price: r.input_price,
          output_price: r.output_price,
          note: r.note,
        })
      }
    }
    editor.loaded = true
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    editor.loading = false
  }
}

async function saveGroup(group: AccountGroup) {
  const editor = editorFor(group)
  const rules: BatchCostRuleRow[] = editor.rows
    .filter((r) => matrixRowConfigured(r))
    .map((r) => ({
      model: r.model,
      billing_mode: r.billing_mode,
      input_price: r.input_price,
      output_price: r.output_price,
      image_price: r.image_price,
      request_price: r.request_price,
      note: r.note,
    }))
  editor.saving = true
  try {
    await replaceCostRules(group.accountId, rules)
    appStore.showSuccess(t('admin.profit.rules.saved'))
    await loadCostRules()
    // 重新对齐编辑器与最新规则
    editor.loaded = false
    await loadEditor(group)
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    editor.saving = false
  }
}

// ─── 新增账号（弹窗选择 → 配置弹窗内配价 → 确认后加入）───
function openPicker() {
  pickerPlatform.value = ''
  pickerGroup.value = ''
  pickerSearch.value = ''
  pickerOpen.value = true
}

function pickAccount(acc: AccountOption) {
  if (isConfigured(acc.id)) return
  pickTarget.value = acc
  pickerOpen.value = false
  void prepareConfig(acc)
}

async function prepareConfig(acc: AccountOption) {
  configRows.value = []
  configOpen.value = true
  configLoading.value = true
  try {
    const modelRes = await getCostRuleModels(acc.id)
    // 若该账号已有历史规则（未配置账号通常没有），回填价格
    const existing = new Map(
      costRules.value
        .filter((r) => (r.account_id ?? null) === acc.id)
        .map((r) => [r.model, r]),
    )
    configRows.value = modelRes.items.map((model) => {
      const r = existing.get(model)
      return {
        model,
        selected: false,
        billing_mode: r?.billing_mode || 'per_image',
        image_price: r?.image_price || 0,
        request_price: r?.request_price || 0,
        input_price: r?.input_price || 0,
        output_price: r?.output_price || 0,
        note: r?.note || '',
      }
    })
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
    configOpen.value = false
  } finally {
    configLoading.value = false
  }
}

async function confirmConfig() {
  const acc = pickTarget.value
  if (!acc) return
  const rules: BatchCostRuleRow[] = configRows.value
    .filter((r) => matrixRowConfigured(r))
    .map((r) => ({
      model: r.model,
      billing_mode: r.billing_mode,
      input_price: r.input_price,
      output_price: r.output_price,
      image_price: r.image_price,
      request_price: r.request_price,
      note: r.note,
    }))
  configSaving.value = true
  try {
    await replaceCostRules(acc.id, rules)
    appStore.showSuccess(t('admin.profit.rules.saved'))
    configOpen.value = false
    await loadCostRules()
    // 加入主列表并展开（展开区会按最新规则重新加载矩阵）
    const key = groupKey(acc.id)
    if (!manualAccountIds.value.includes(acc.id)) manualAccountIds.value.push(acc.id)
    editors[key] = { rows: [], loading: false, saving: false, loaded: false }
    expanded[key] = true
    const group = groups.value.find((g) => g.key === key)
    if (group) await loadEditor(group)
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    configSaving.value = false
  }
}

function matrixRowConfigured(row: CostRuleMatrixRow): boolean {
  if (row.billing_mode === 'per_image') return row.image_price > 0
  if (row.billing_mode === 'per_request') return row.request_price > 0
  return row.input_price > 0 || row.output_price > 0
}

// ─── 清空某账号规则 ───
const clearTargetName = computed(() => {
  const group = groups.value.find((g) => g.key === clearTargetKey.value)
  if (!group) return ''
  return group.accountId === null ? t('admin.profit.rules.anyAccount') : (group.name || `#${group.accountId}`)
})

function confirmClear(group: AccountGroup) {
  clearTargetKey.value = group.key
  clearConfirmShow.value = true
}

async function doClearGroup() {
  clearConfirmShow.value = false
  const group = groups.value.find((g) => g.key === clearTargetKey.value)
  if (!group) return
  const editor = editorFor(group)
  editor.saving = true
  try {
    await replaceCostRules(group.accountId, [])
    appStore.showSuccess(t('admin.profit.rules.saved'))
    await loadCostRules()
    editor.rows = []
    editor.loaded = false
    expanded[group.key] = false
    // 已无规则且是手动加进来的分组，从列表移除
    manualAccountIds.value = manualAccountIds.value.filter((id) => groupKey(id) !== group.key)
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    editor.saving = false
  }
}

onMounted(async () => {
  await Promise.all([reload(), loadAccountOptions(), loadGroupOptions()])
})
</script>
