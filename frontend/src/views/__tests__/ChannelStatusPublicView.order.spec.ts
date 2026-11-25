import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import ChannelStatusPublicView from '../ChannelStatusPublicView.vue'

const mocks = vi.hoisted(() => ({ getStatus: vi.fn() }))

vi.mock('@/api/channelStatus', () => ({ getChannelStatus: mocks.getStatus, getChannelStatusAnnouncement: vi.fn() }))
vi.mock('@/api/admin/settings', () => ({ getChannelStatusAccountStats: vi.fn(), getChannelStatusSnapshot: vi.fn() }))
vi.mock('@/api/admin/accounts', () => ({ accountsAPI: { list: vi.fn() } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ fetchPublicSettings: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: false }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

enableAutoUnmount(afterEach)

const PassThroughTransition = defineComponent({ name: 'Transition', setup: (_props, { slots }) => () => slots.default?.() })

function group(id: number, status: string, successRate: number) {
  const latency = { avg_gen_ms: 1000, fastest_ms: 500, slowest_ms: 2000, p95_ms: 1500, p99_ms: 1800 }
  return {
    id, name: `group-${id}`, platform: 'openai', status, requests: 10, failures: 0, success_rate: successRate,
    ...latency, models: [], timeline: [],
  }
}

// 三个状态各异的分组，快照顺序故意与「故障优先」相反。
const snapshotGroups = () => [
  group(1, 'operational', 100),
  group(2, 'degraded', 80),
  group(3, 'down', 40),
]

function snapshot(orderConfigured: boolean) {
  return {
    generated_at_ms: 1, window_hours: 24, refresh_seconds: 60, overall: 'down',
    thresholds: { healthy: 90, degraded: 70 }, order_configured: orderConfigured,
    avg_gen_ms: 1000, fastest_ms: 500, slowest_ms: 2000, p95_ms: 1500, p99_ms: 1800,
    groups: snapshotGroups(),
  }
}

async function mountReady(orderConfigured: boolean) {
  mocks.getStatus.mockResolvedValue(snapshot(orderConfigured))
  const wrapper = shallowMount(ChannelStatusPublicView, {
    global: { stubs: { transition: PassThroughTransition } },
  })
  await flushPromises()
  return wrapper
}

function renderedGroupIds(wrapper: ReturnType<typeof shallowMount>) {
  return wrapper
    .findAll('article[data-testid^="status-group-"]')
    .map((card) => Number(card.attributes('data-testid')?.replace('status-group-', '')))
}

beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear()
})

describe('channel status group order', () => {
  it('keeps the admin-defined order instead of sorting by health', async () => {
    const wrapper = await mountReady(true)
    expect(renderedGroupIds(wrapper)).toEqual([1, 2, 3])
  })

  it('falls back to worst-first when no order is configured', async () => {
    const wrapper = await mountReady(false)
    expect(renderedGroupIds(wrapper)).toEqual([3, 2, 1])
  })
})
