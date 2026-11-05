<template>
  <BaseDialog
    :show="show"
    :title="t('admin.usage.attemptChain.title')"
    width="full"
    @close="close"
  >
    <div class="space-y-3">
      <div class="flex flex-wrap items-center gap-2">
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.usage.attemptChain.hint', { count: attempts.length }) }}
        </p>
        <span
          v-if="requestId"
          class="truncate rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400"
          :title="requestId"
        >{{ requestId }}</span>
      </div>

      <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">#</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.attemptChain.account') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.attemptChain.outcome') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.attemptChain.duration') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.attemptChain.error') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
            <tr v-for="(attempt, index) in attempts" :key="`${attempt.account_id}-${index}`">
              <td class="whitespace-nowrap px-3 py-2 text-xs tabular-nums text-gray-500 dark:text-gray-400">{{ index + 1 }}</td>
              <td class="whitespace-nowrap px-3 py-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ attempt.account_name || `#${attempt.account_id}` }}</span>
                <span
                  v-if="attempt.platform"
                  class="ml-1.5 inline-flex items-center rounded px-1 py-px text-[10px] font-medium ring-1 ring-inset bg-gray-100 text-gray-600 ring-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:ring-dark-600"
                >{{ attempt.platform }}</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2">
                <span
                  v-if="attempt.success"
                  class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-green-100 text-green-800 ring-1 ring-inset ring-green-200 dark:bg-green-500/20 dark:text-green-400 dark:ring-green-500/30"
                >{{ t('admin.usage.attemptChain.success') }}</span>
                <span
                  v-else
                  class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ring-1 ring-inset"
                  :class="statusCodeBadgeClass(attempt.status_code ?? 0)"
                >
                  {{ attempt.status_code && attempt.status_code > 0 ? attempt.status_code : t('admin.usage.attemptChain.unknownStatus') }}
                </span>
              </td>
              <td class="whitespace-nowrap px-3 py-2 text-xs tabular-nums text-gray-700 dark:text-gray-300">{{ formatDuration(attempt.duration_ms) }}</td>
              <td class="px-3 py-2">
                <span
                  v-if="attempt.error"
                  class="block max-w-md break-all font-mono text-xs text-rose-600 dark:text-rose-400"
                  :title="attempt.error"
                >{{ attempt.error }}</span>
                <span v-else class="text-xs text-gray-400 dark:text-gray-500">-</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <button
        type="button"
        class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
        @click="close"
      >
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { statusCodeBadgeClass } from '@/utils/errorBadges'
import type { AdminUsageAccountAttempt } from '@/types'

defineProps<{
  show: boolean
  attempts: AdminUsageAccountAttempt[]
  requestId?: string | null
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const { t } = useI18n()

const close = () => emit('update:show', false)

const formatDuration = (ms?: number) => {
  if (ms == null) return '-'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}
</script>
