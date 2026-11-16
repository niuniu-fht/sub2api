<template>
  <div>
    <!-- 统一设置条 -->
    <div class="mb-2 flex flex-wrap items-end gap-2 rounded-xl bg-white p-3 shadow-sm dark:bg-dark-800">
      <label class="flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
        <input
          type="checkbox"
          class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 dark:border-dark-500"
          :checked="allSelected"
          @change="selectAll(($event.target as HTMLInputElement).checked)"
        />
        {{ t('admin.profit.rules.selectAll') }}
      </label>
      <span class="text-xs text-gray-500">{{ t('admin.profit.rules.selectedCount', { n: selectedCount }) }}</span>
      <div class="ml-auto flex flex-wrap items-end gap-2">
        <div>
          <label class="text-xs text-gray-500">{{ t('admin.profit.rules.mode') }}</label>
          <select
            v-model="applyMode"
            class="mt-0.5 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
          >
            <option value="per_image">{{ t('admin.profit.rules.mode_per_image') }}</option>
            <option value="per_request">{{ t('admin.profit.rules.mode_per_request') }}</option>
            <option value="token">{{ t('admin.profit.rules.mode_token') }}</option>
          </select>
        </div>
        <div>
          <label class="text-xs text-gray-500">{{ t('admin.profit.rules.price') }}</label>
          <input
            v-model.number="applyPrice"
            type="number" step="0.001" min="0"
            class="mt-0.5 w-24 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
          />
        </div>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="!selectedCount || !applyPrice"
          @click="applyToSelected"
        >
          {{ t('admin.profit.rules.applySelected', { n: selectedCount }) }}
        </button>
      </div>
    </div>

    <!-- 模型 × 价格矩阵 -->
    <div class="max-h-[46vh] overflow-y-auto rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
      <table class="min-w-full text-sm">
        <thead class="sticky top-0 bg-gray-50 dark:bg-dark-900">
          <tr class="border-b border-gray-200 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:text-gray-400">
            <th class="px-3 py-2 w-10"></th>
            <th class="px-3 py-2">{{ t('admin.profit.rules.model') }}</th>
            <th class="px-3 py-2 w-28">{{ t('admin.profit.rules.mode') }}</th>
            <th class="px-3 py-2 w-40">{{ t('admin.profit.rules.price') }}</th>
            <th class="px-3 py-2">{{ t('admin.profit.rules.note') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.model" class="border-b border-gray-100 dark:border-dark-700/50">
            <td class="px-3 py-2">
              <input
                v-model="row.selected"
                type="checkbox"
                class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 dark:border-dark-500"
              />
            </td>
            <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">{{ row.model }}</td>
            <td class="px-3 py-2">
              <select
                v-model="row.billing_mode"
                class="w-full rounded-lg border border-gray-300 bg-white px-1.5 py-1 text-xs text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              >
                <option value="per_image">{{ t('admin.profit.rules.mode_per_image') }}</option>
                <option value="per_request">{{ t('admin.profit.rules.mode_per_request') }}</option>
                <option value="token">{{ t('admin.profit.rules.mode_token') }}</option>
              </select>
            </td>
            <td class="px-3 py-2">
              <input
                v-if="row.billing_mode === 'per_image'" v-model.number="row.image_price"
                type="number" step="0.001" min="0"
                class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1 text-xs tabular-nums text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                :class="rowConfigured(row) ? '' : 'border-dashed dark:border-dark-500'"
              />
              <input
                v-else-if="row.billing_mode === 'per_request'" v-model.number="row.request_price"
                type="number" step="0.001" min="0"
                class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1 text-xs tabular-nums text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                :class="rowConfigured(row) ? '' : 'border-dashed dark:border-dark-500'"
              />
              <div v-else class="flex gap-1">
                <input v-model.number="row.input_price" type="number" step="0.01" min="0" placeholder="in ¥/M"
                  class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1 text-xs tabular-nums text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
                <input v-model.number="row.output_price" type="number" step="0.01" min="0" placeholder="out ¥/M"
                  class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1 text-xs tabular-nums text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
              </div>
            </td>
            <td class="px-3 py-2">
              <input v-model="row.note" type="text"
                class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1 text-xs text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
            </td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="5" class="px-3 py-8 text-center text-gray-400">{{ t('admin.profit.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="mt-2 text-xs text-gray-400">{{ t('admin.profit.rules.zeroHint') }}</p>
  </div>
</template>

<script lang="ts">
export interface CostRuleMatrixRow {
  model: string
  selected: boolean
  billing_mode: 'token' | 'per_image' | 'per_request'
  image_price: number
  request_price: number
  input_price: number
  output_price: number
  note: string
}
</script>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{ rows: CostRuleMatrixRow[] }>()

const applyMode = ref<'per_image' | 'per_request' | 'token'>('per_image')
const applyPrice = ref<number | null>(null)

const selectedCount = computed(() => props.rows.filter((r) => r.selected).length)
const allSelected = computed(() => props.rows.length > 0 && selectedCount.value === props.rows.length)

function selectAll(v: boolean) {
  props.rows.forEach((r) => { r.selected = v })
}

function rowConfigured(row: CostRuleMatrixRow): boolean {
  if (row.billing_mode === 'per_image') return row.image_price > 0
  if (row.billing_mode === 'per_request') return row.request_price > 0
  return row.input_price > 0 || row.output_price > 0
}

function applyToSelected() {
  const price = applyPrice.value
  if (price == null || price <= 0) return
  props.rows.forEach((r) => {
    if (!r.selected) return
    r.billing_mode = applyMode.value
    if (applyMode.value === 'per_image') r.image_price = price
    else if (applyMode.value === 'per_request') r.request_price = price
    else { r.input_price = price; r.output_price = 0 }
  })
}

defineExpose({ rowConfigured })
</script>
