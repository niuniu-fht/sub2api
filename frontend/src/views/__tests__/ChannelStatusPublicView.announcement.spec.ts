import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import ChannelStatusPublicView from '../ChannelStatusPublicView.vue'

const mocks = vi.hoisted(() => ({ getStatus: vi.fn(), getAnnouncement: vi.fn(), isAdmin: false }))

vi.mock('@/api/channelStatus', () => ({
  getChannelStatus: mocks.getStatus,
  getChannelStatusAnnouncement: mocks.getAnnouncement
}))
vi.mock('@/api/admin/settings', () => ({ getChannelStatusAccountStats: vi.fn(), getChannelStatusSnapshot: vi.fn() }))
vi.mock('@/api/admin/accounts', () => ({ accountsAPI: { list: vi.fn() } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ fetchPublicSettings: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ get isAdmin() { return mocks.isAdmin } }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

enableAutoUnmount(afterEach)
afterEach(() => localStorage.clear())

const DISMISS_KEY = 'sub2api:channel-status-ann-dismissed'
const BAR = '[data-testid="status-announcement"]'
const CLOSE = '[data-testid="status-announcement-close"]'
const EDIT = '[data-testid="status-announcement-edit"]'

function snapshot() {
  return {
    generated_at_ms: 1,
    window_hours: 24,
    refresh_seconds: 60,
    overall: 'operational',
    thresholds: { healthy: 90, degraded: 70 },
    avg_gen_ms: 1000,
    fastest_ms: 500,
    slowest_ms: 2000,
    p95_ms: 1500,
    p99_ms: 1800,
    groups: [{
      id: 20, name: '低质量', platform: 'openai', status: 'operational', requests: 10, failures: 0,
      success_rate: 100, avg_gen_ms: 1000, fastest_ms: 500, slowest_ms: 2000, p95_ms: 1500, p99_ms: 1800,
      models: [], timeline: []
    }]
  }
}

let resolveWarnings: string[] = []

const PassThroughTransition = defineComponent({
  name: 'Transition',
  setup: (_props, { slots }) => () => slots.default?.()
})

function mountView(props: Record<string, unknown> = {}) {
  resolveWarnings = []
  return shallowMount(ChannelStatusPublicView, {
    props,
    global: {
      // 通栏包在 <Transition> 里；换成直通桩，断言不受离场动画时序影响。
      stubs: { transition: PassThroughTransition },
      config: {
        // 模板里用到却未 import/注册的组件（例如 Icon）只会静默渲染成空元素，
        // 页面上表现为"按钮在但没有图标"。这里把这类警告收集起来统一断言。
        warnHandler: (message: string) => { if (message.includes('Failed to resolve component')) resolveWarnings.push(message) }
      }
    }
  })
}

async function mountReady(props: Record<string, unknown> = {}) {
  mocks.getStatus.mockResolvedValue(snapshot())
  const wrapper = mountView(props)
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.isAdmin = false
  localStorage.clear()
  mocks.getAnnouncement.mockResolvedValue({ content: '今晚 02:00 例行维护', updated_at_ms: 1000 })
})

describe('channel status announcement bar', () => {
  it('renders as a sticky full-width bar at the very top of the page', async () => {
    const wrapper = await mountReady()
    // 通栏是根节点的第一个子元素（位于标题与分组列表之前），吸顶悬浮。
    expect(wrapper.element.firstElementChild?.getAttribute('data-testid')).toBe('status-announcement')
    const bar = wrapper.get(BAR)
    expect(bar.classes()).toContain('sticky')
    expect(bar.classes()).toContain('top-0')
    expect(bar.text()).toContain('今晚 02:00 例行维护')
    expect(resolveWarnings).toEqual([])
  })

  it('hides immediately when closed and remembers the dismissal', async () => {
    const wrapper = await mountReady()
    expect(wrapper.get(CLOSE).text()).toContain('common.close')
    await wrapper.get(CLOSE).trigger('click')
    await flushPromises()
    // 回归：关闭状态曾存进非响应式的 localStorage 读取，点击后通栏不消失。
    expect(wrapper.find(BAR).exists()).toBe(false)
    expect(localStorage.getItem(DISMISS_KEY)).toBe('1000')
  })

  it('stays hidden for a dismissed announcement until a newer one is published', async () => {
    localStorage.setItem(DISMISS_KEY, '1000')
    const dismissed = await mountReady()
    expect(dismissed.find(BAR).exists()).toBe(false)

    mocks.getAnnouncement.mockResolvedValue({ content: '维护已提前完成', updated_at_ms: 2000 })
    const renewed = await mountReady()
    expect(renewed.get(BAR).text()).toContain('维护已提前完成')
  })

  it('is not rendered when there is no announcement', async () => {
    mocks.getAnnouncement.mockResolvedValue({ content: '', updated_at_ms: 0 })
    const wrapper = await mountReady()
    expect(wrapper.find(BAR).exists()).toBe(false)
  })

  it('cancels the layout padding only when embedded in the app layout', async () => {
    const anonymous = await mountReady()
    expect(anonymous.get(BAR).classes()).not.toContain('-mx-4')

    const panel = await mountReady({ panel: true })
    expect(panel.get(BAR).classes()).toEqual(expect.arrayContaining(['-mx-4', '-mt-4', 'md:-mx-6', 'lg:-mx-8', 'lg:-mt-8']))
  })

  it('offers editing only to admins', async () => {
    const user = await mountReady({ panel: true })
    expect(user.find(EDIT).exists()).toBe(false)

    mocks.isAdmin = true
    const admin = await mountReady({ panel: true })
    expect(admin.get(EDIT).text()).toContain('statusPage.annEdit')
    // 有生效公告时喇叭按钮右上角要有红点（曾因模板改坏只剩一个空元素）
    expect(admin.get('[data-testid="status-announcement-btn"] .bg-red-500').exists()).toBe(true)
  })
})
