<template>
  <div
    v-if="open"
    class="fixed inset-0 z-[70] flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
    @click.self="close"
  >
    <div class="w-full max-w-lg rounded-2xl border border-gray-200 bg-white p-5 shadow-xl dark:border-dark-700 dark:bg-dark-800">
      <div class="mb-3 flex items-center gap-2">
        <span class="flex h-8 w-8 items-center justify-center rounded-xl bg-amber-50 text-amber-500 dark:bg-amber-500/10 dark:text-amber-400">
          <Icon name="megaphone" size="md" />
        </span>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('statusPage.annDialogTitle') }}
        </h3>
      </div>
      <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
        {{ t('statusPage.annDialogHint') }}
      </p>
      <textarea
        v-model="draft"
        rows="5"
        maxlength="2000"
        class="input w-full resize-y font-normal"
        :placeholder="t('statusPage.annPlaceholder')"
      ></textarea>
      <div class="mt-1 text-right text-[10px] text-gray-400">{{ draft.length }} / 2000</div>
      <div v-if="error" class="mt-2 text-xs text-red-500">{{ error }}</div>
      <div class="mt-4 flex items-center justify-between gap-3">
        <button
          v-if="currentContent"
          type="button"
          class="btn btn-secondary text-red-500"
          :disabled="saving"
          @click="clearAnnouncement"
        >{{ t('statusPage.annClear') }}</button>
        <span v-else></span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary" @click="close">{{ t('common.cancel') }}</button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="saving || draft.trim() === currentContent"
            @click="save"
          >{{ saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  getChannelStatusAnnouncementAdmin,
  updateChannelStatusAnnouncement,
} from '@/api/admin/settings'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void; (e: 'saved', content: string, updatedAtMs: number): void }>()

const { t } = useI18n()
const draft = ref('')
const currentContent = ref('')
const saving = ref(false)
const error = ref('')

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    error.value = ''
    try {
      const ann = await getChannelStatusAnnouncementAdmin()
      currentContent.value = ann.content || ''
      draft.value = ann.content || ''
    } catch {
      currentContent.value = ''
      draft.value = ''
    }
  },
  { immediate: true },
)

function close(): void {
  emit('update:open', false)
}

async function save(): Promise<void> {
  saving.value = true
  error.value = ''
  try {
    const ann = await updateChannelStatusAnnouncement(draft.value)
    currentContent.value = ann.content || ''
    emit('saved', ann.content || '', ann.updated_at_ms || 0)
    close()
  } catch (e: any) {
    error.value = e?.message || t('common.operationFailed')
  } finally {
    saving.value = false
  }
}

async function clearAnnouncement(): Promise<void> {
  saving.value = true
  error.value = ''
  try {
    await updateChannelStatusAnnouncement('')
    currentContent.value = ''
    draft.value = ''
    emit('saved', '', 0)
    close()
  } catch (e: any) {
    error.value = e?.message || t('common.operationFailed')
  } finally {
    saving.value = false
  }
}
</script>
