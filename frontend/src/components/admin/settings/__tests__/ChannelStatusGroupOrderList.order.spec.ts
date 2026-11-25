import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import ChannelStatusGroupOrderList from '../ChannelStatusGroupOrderList.vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

// jsdom 里无法真实拖动：换成直通桩，这里只验证顺序与按钮逻辑。
vi.mock('vue-draggable-plus', () => ({
  VueDraggable: defineComponent({ name: 'VueDraggable', setup: (_props, { slots }) => () => slots.default?.() }),
}))

const options = [
  { id: 20, name: '低质量', platform: 'openai' },
  { id: 32, name: '中质量', platform: 'openai' },
  { id: 40, name: '高质量', platform: 'openai' },
]

function mountList(ids: number[]) {
  return mount(ChannelStatusGroupOrderList, {
    props: { modelValue: ids, options },
    global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { en, zh } })] },
  })
}

const item = (id: number) => `[data-testid="channel-status-order-item-${id}"]`
const control = (action: 'top' | 'up' | 'down', id: number) => `[data-testid="channel-status-order-${action}-${id}"]`

function renderedIds(wrapper: ReturnType<typeof mountList>): number[] {
  return wrapper
    .findAll('[data-testid^="channel-status-order-item-"]')
    .map((element) => Number(element.attributes('data-testid')?.replace('channel-status-order-item-', '')))
}

describe('channel status group display order', () => {
  it('lists the selected groups in modelValue order with names and positions', () => {
    const wrapper = mountList([40, 20, 32])
    expect(renderedIds(wrapper)).toEqual([40, 20, 32])
    expect(wrapper.get(item(40)).text()).toContain('高质量')
    const positions = wrapper.findAll('[data-testid^="channel-status-order-item-"] .tabular-nums').map((el) => el.text())
    expect(positions).toEqual(['1', '2', '3'])
  })

  it('moves a group to the top and emits the new order', async () => {
    const wrapper = mountList([40, 20, 32])
    await wrapper.get(control('top', 32)).trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[32, 40, 20]])
  })

  it('swaps with the neighbour on up and down', async () => {
    const wrapper = mountList([40, 20, 32])
    await wrapper.get(control('up', 20)).trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[20, 40, 32]])

    await wrapper.get(control('down', 20)).trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[40, 32, 20]])
  })

  it('offers no move-up or top on the first row and no move-down on the last row', () => {
    const wrapper = mountList([40, 20, 32])
    expect(wrapper.find(control('top', 40)).exists()).toBe(false)
    expect(wrapper.find(control('up', 40)).exists()).toBe(false)
    expect(wrapper.find(control('down', 32)).exists()).toBe(false)
    expect(wrapper.find(control('top', 20)).exists()).toBe(true)
    expect(wrapper.find(control('down', 20)).exists()).toBe(true)
  })

  it('falls back to the id when the group is unknown', () => {
    const wrapper = mountList([99])
    expect(wrapper.get(item(99)).text()).toContain('#99')
  })
})
