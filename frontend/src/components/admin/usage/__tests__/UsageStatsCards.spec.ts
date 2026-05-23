import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'

const messages: Record<string, string> = {
  'usage.totalRequests': '总请求数',
  'usage.inSelectedRange': '所选范围内',
  'usage.totalTokens': '总 Token',
  'usage.in': '输入',
  'usage.out': '输出',
  'usage.totalCost': '总消费',
  'usage.accountCost': '成本',
  'usage.standardCost': '标准',
  'usage.profit': '利润',
  'usage.avgDuration': '平均耗时',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

describe('admin UsageStatsCards', () => {
  it('shows profit as total consumption minus account cost', () => {
    const stats: AdminUsageStatsResponse = {
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 1180.7903,
      total_actual_cost: 3246.9265,
      total_account_cost: 2331.6422,
      average_duration_ms: 0,
    }

    const wrapper = mount(UsageStatsCards, {
      props: { stats },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('总消费 $3246.9265')
    expect(wrapper.text()).toContain('成本 $2331.6422')
    expect(wrapper.text()).toContain('利润 $915.2843')
    expect(wrapper.text()).not.toContain('标准 $1180.7903')
  })
})
