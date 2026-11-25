<template>
  <div data-testid="channel-status-display-order">
    <label class="input-label mb-1">{{ t('admin.settings.channelStatus.displayOrder') }}</label>
    <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
      {{ t('admin.settings.channelStatus.displayOrderHint') }}
    </p>
    <VueDraggable
      v-model="orderedGroups"
      :animation="200"
      handle=".drag-handle"
      class="space-y-1.5"
      data-testid="channel-status-order-list"
    >
      <div
        v-for="(group, index) in orderedGroups"
        :key="group.id"
        class="flex items-center gap-2.5 rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-xs dark:border-dark-600 dark:bg-dark-900"
        :data-testid="`channel-status-order-item-${group.id}`"
      >
        <span class="drag-handle flex cursor-grab items-center text-gray-400 hover:text-gray-600 active:cursor-grabbing dark:text-dark-600 dark:hover:text-gray-300">
          <Icon name="menu" size="sm" />
        </span>
        <span class="w-5 shrink-0 text-center font-semibold tabular-nums text-gray-400 dark:text-gray-500">{{ index + 1 }}</span>
        <span class="min-w-0 flex-1 truncate font-medium text-gray-900 dark:text-gray-100">{{ group.name }}</span>
        <button
          v-if="index > 0"
          type="button"
          class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
          :data-testid="`channel-status-order-top-${group.id}`"
          @click="moveToTop(group.id)"
        >
          {{ t('admin.settings.channelStatus.moveToTop') }}
        </button>
        <button
          v-if="index > 0"
          type="button"
          class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
          :title="t('admin.settings.channelStatus.moveUp')"
          :aria-label="t('admin.settings.channelStatus.moveUp')"
          :data-testid="`channel-status-order-up-${group.id}`"
          @click="move(group.id, -1)"
        >
          <Icon name="arrowUp" size="sm" />
        </button>
        <button
          v-if="index < orderedGroups.length - 1"
          type="button"
          class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
          :title="t('admin.settings.channelStatus.moveDown')"
          :aria-label="t('admin.settings.channelStatus.moveDown')"
          :data-testid="`channel-status-order-down-${group.id}`"
          @click="move(group.id, 1)"
        >
          <Icon name="arrowDown" size="sm" />
        </button>
      </div>
    </VueDraggable>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'

import Icon from '@/components/icons/Icon.vue'

/**
 * 渠道状态页的分组展示顺序编辑器：modelValue 是分组 ID 数组，数组顺序即状态页从上到下的
 * 展示顺序。拖拽（VueDraggable）与「置顶 / 上移 / 下移」按钮都只是重排这个数组，
 * 由父组件随整份统计配置一起保存。
 */
const props = defineProps<{
  modelValue: number[]
  /** 可选分组（用于把 ID 显示成名称） */
  options: Array<{ id: number; name: string; platform: string }>
}>()
const emit = defineEmits<{ (e: 'update:modelValue', ids: number[]): void }>()

const { t } = useI18n()

type OrderedGroup = { id: number; name: string }

const orderedGroups = computed<OrderedGroup[]>({
  get: () =>
    props.modelValue.map((id) => ({
      id,
      name: props.options.find((group) => group.id === id)?.name ?? `#${id}`,
    })),
  set: (groups) => emit('update:modelValue', groups.map((group) => group.id)),
})

function move(id: number, offset: -1 | 1): void {
  const ids = props.modelValue
  const from = ids.indexOf(id)
  const to = from + offset
  if (from < 0 || to < 0 || to >= ids.length) {
    return
  }
  const moved = ids.filter((other) => other !== id)
  moved.splice(to, 0, id)
  emit('update:modelValue', moved)
}

function moveToTop(id: number): void {
  if (props.modelValue[0] === id) {
    return
  }
  emit('update:modelValue', [id, ...props.modelValue.filter((other) => other !== id)])
}
</script>
