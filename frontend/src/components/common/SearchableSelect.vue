<template>
  <div class="relative" :data-testid="testId">
    <button
      type="button"
      class="flex w-full items-center justify-between gap-2 rounded-lg border border-gray-300 bg-white px-3 py-2 text-left text-sm text-gray-900 transition hover:border-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white dark:hover:border-dark-500"
      @click="toggle"
    >
      <span class="min-w-0 flex-1 truncate" :class="selected ? '' : 'text-gray-400 dark:text-gray-500'">
        {{ selected ? selected.label : placeholder }}
      </span>
      <svg
        class="h-4 w-4 shrink-0 text-gray-400 transition-transform"
        :class="open ? 'rotate-180' : ''"
        viewBox="0 0 20 20"
        fill="currentColor"
      >
        <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.17l3.71-3.94a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
      </svg>
    </button>

    <div
      v-if="open"
      class="absolute z-30 mt-1 w-full overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
    >
      <div class="flex items-center gap-2 border-b border-gray-100 px-3 py-2 dark:border-dark-700">
        <svg class="h-4 w-4 shrink-0 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.45 4.39l3.08 3.08a.75.75 0 11-1.06 1.06l-3.08-3.08A7 7 0 012 9z" clip-rule="evenodd" />
        </svg>
        <input
          ref="searchRef"
          v-model="search"
          type="text"
          class="flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-white"
          :placeholder="searchPlaceholder"
        />
      </div>
      <div class="max-h-56 overflow-y-auto py-1">
        <button
          v-for="opt in filtered"
          :key="String(opt.value)"
          type="button"
          class="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm transition"
          :class="opt.value === modelValue
            ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200'
            : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-700'"
          @click="choose(opt.value)"
        >
          <span class="min-w-0 flex-1 truncate">{{ opt.label }}</span>
          <span v-if="opt.hint" class="shrink-0 text-xs text-gray-400">{{ opt.hint }}</span>
        </button>
        <div v-if="!filtered.length" class="px-3 py-4 text-center text-sm text-gray-400">
          {{ emptyText }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'

export interface SelectOption {
  value: string | number | null
  label: string
  hint?: string
}

interface Props {
  modelValue: string | number | null
  options: SelectOption[]
  placeholder?: string
  searchPlaceholder?: string
  emptyText?: string
  testId?: string
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: '—',
  searchPlaceholder: '搜索…',
  emptyText: '无选项',
  testId: '',
})

const emit = defineEmits<{ (e: 'update:modelValue', v: string | number | null): void }>()

const open = ref(false)
const search = ref('')
const searchRef = ref<HTMLInputElement | null>(null)

const selected = computed(() => props.options.find((o) => o.value === props.modelValue) || null)

const filtered = computed(() => {
  const kw = search.value.trim().toLowerCase()
  if (!kw) return props.options
  return props.options.filter(
    (o) => o.label.toLowerCase().includes(kw) || (o.hint || '').toLowerCase().includes(kw),
  )
})

function toggle() {
  open.value = !open.value
  if (open.value) {
    search.value = ''
    nextTick(() => searchRef.value?.focus())
  }
}

function choose(v: string | number | null) {
  emit('update:modelValue', v)
  open.value = false
}

function onDocClick(e: Event) {
  if (!open.value) return
  const el = e.target as HTMLElement
  if (el.closest(`[data-testid="${props.testId || 'searchable-select'}"]`)) return
  open.value = false
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', onDocClick)
  onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
}
</script>
