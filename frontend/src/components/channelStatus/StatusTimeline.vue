<template>
  <div class="relative" @mouseleave="hovered = null">
    <!-- 时间线条（最旧在左，最新在右）；点击可下钻该时段的模型明细 -->
    <div class="flex h-5 w-full items-stretch gap-[3px]">
      <div
        v-for="(bucket, idx) in buckets"
        :key="idx"
        class="min-w-0 flex-1 cursor-pointer rounded-sm transition-all duration-200 hover:scale-y-110"
        :class="[STATUS_BAR[bucket.status] ?? STATUS_BAR.unknown, selectedIndex === idx ? selectedRing : '']"
        :data-testid="`status-bar-${idx}`"
        @mouseenter="hovered = idx"
        @click.stop="emit('select', idx)"
      ></div>
    </div>

    <!-- 悬浮信息气泡（组件化，替代原生 title） -->
    <Transition
      enter-active-class="transition-opacity duration-150 ease-out"
      enter-from-class="opacity-0"
      leave-active-class="transition-opacity duration-100 ease-in"
      leave-to-class="opacity-0"
    >
      <div
        v-if="hovered !== null && buckets[hovered]"
        class="pointer-events-none absolute bottom-full z-20 mb-2 -translate-x-1/2"
        :style="{ left: bubbleLeftPct(hovered) + '%' }"
        data-testid="status-bubble"
      >
        <div class="rounded-lg bg-gray-900 px-3 py-2 text-xs leading-5 text-white shadow-xl ring-1 ring-white/10 dark:bg-dark-700 dark:ring-white/10">
          <div class="font-medium">{{ bubbleTime }}</div>
          <div class="mt-0.5 flex items-center gap-1.5 whitespace-nowrap">
            <span class="h-1.5 w-1.5 rounded-full" :class="STATUS_DOT[bucketStatus]"></span>
            {{ bubbleStatusLabel }}
            <span v-if="bucketRate !== '—'" class="tabular-nums opacity-80">· {{ bucketRate }}</span>
          </div>
        </div>
        <!-- 箭头 -->
        <div class="mx-auto h-2 w-2 -translate-y-1 rotate-45 bg-gray-900 dark:bg-dark-700"></div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChannelHealthStatus, ChannelStatusBucket } from '@/api/channelStatus'
import { formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{
  buckets: ChannelStatusBucket[]
  /** 当前被选中（下钻）的桶下标；null 表示未选中任何桶。 */
  selectedIndex?: number | null
}>()

const emit = defineEmits<{
  (e: 'select', index: number): void
}>()

const { t } = useI18n()
const hovered = ref<number | null>(null)

// 选中桶的高亮环：叠加在状态色之上，标示「模型明细当前展示的时段」。
const selectedRing = 'ring-2 ring-gray-900 dark:ring-white/80 ring-offset-1 ring-offset-white dark:ring-offset-dark-800'

const STATUS_DOT: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  down: 'bg-red-500',
  unknown: 'bg-gray-300 dark:bg-dark-600'
}

const STATUS_BAR: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  down: 'bg-red-500',
  unknown: 'bg-gray-200 dark:bg-dark-700'
}

// 气泡水平位置跟随条块中心，但向内收拢避免两端溢出卡片。
function bubbleLeftPct(idx: number): number {
  const raw = ((idx + 0.5) / Math.max(props.buckets.length, 1)) * 100
  return Math.min(90, Math.max(10, raw))
}

const activeBucket = computed(() => (hovered.value !== null ? props.buckets[hovered.value] : undefined))

const bubbleTime = computed(() => {
  const bucket = activeBucket.value
  if (!bucket) return ''
  return formatDateTimeToMinute(new Date(bucket.bucket_start_ms))
})

const bucketStatus = computed(() => activeBucket.value?.status ?? 'unknown')

const bubbleStatusLabel = computed(() => {
  const status = bucketStatus.value
  return t(`statusPage.status${status.charAt(0).toUpperCase()}${status.slice(1)}`)
})

const bucketRate = computed(() => {
  const bucket = activeBucket.value
  if (!bucket || bucket.success_rate < 0) return '—'
  return `${Math.round(bucket.success_rate * 100) / 100}%`
})
</script>
